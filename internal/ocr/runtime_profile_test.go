package ocr

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func runtimeProfileFixture(t *testing.T) (string, ocrRuntimeProfile, map[string][]byte) {
	t.Helper()
	source := t.TempDir()
	assets := map[string][]byte{
		"RapidOCR-json.exe": []byte("stock fixture executable"),
		"det.onnx":          {0x08, 8, 0x12, 3, 'd', 'e', 't'},
		"cls.onnx":          {0x08, 8, 0x12, 3, 'c', 'l', 's'},
		"rec.onnx":          {0x08, 8, 0x12, 3, 'r', 'e', 'c'},
		"keys.txt":          []byte("a\nb\n中\n"),
	}
	profile := ocrRuntimeProfile{}
	for _, name := range []string{"RapidOCR-json.exe", "det.onnx", "cls.onnx", "rec.onnx", "keys.txt"} {
		data := assets[name]
		digest := sha256.Sum256(data)
		asset := ocrRuntimeAsset{name: name, size: int64(len(data)), digest: hex.EncodeToString(digest[:]), model: strings.HasSuffix(name, ".onnx")}
		path := filepath.Join(source, "models", name)
		if name == "RapidOCR-json.exe" {
			profile.executable = asset
			path = filepath.Join(source, name)
		} else {
			profile.files = append(profile.files, asset)
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, data, 0600))
	}
	return source, profile, assets
}

func runtimeProfileCommand(source string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(source, "RapidOCR-json.exe"), "--ensureAscii=1")
	cmd.Dir = source
	return cmd
}

func runtimeProfileModelsPath(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	require.Equal(t, "--models=.", cmd.Args[len(cmd.Args)-1])
	require.True(t, filepath.IsAbs(cmd.Dir))
	return cmd.Dir
}

func TestOCRRuntimeProfileCopiesOnlyVerifiedModelsAndPreservesSources(t *testing.T) {
	source, profile, assets := runtimeProfileFixture(t)
	cache := t.TempDir()
	cmd := runtimeProfileCommand(source)
	cmd.Env = []string{"CUSTOM=unchanged", "ort_load_config_from_model=0", "WITH_EQUALS=a=b"}
	applied, err := applyOCRRuntimeProfile(cmd, cache, profile)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, []string{"CUSTOM=unchanged", "WITH_EQUALS=a=b", "ORT_LOAD_CONFIG_FROM_MODEL=1"}, cmd.Env)
	models := runtimeProfileModelsPath(t, cmd)
	for name, original := range assets {
		path := filepath.Join(source, "models", name)
		if name == profile.executable.name {
			path = filepath.Join(source, name)
		} else {
			cached, err := os.ReadFile(filepath.Join(models, name))
			require.NoError(t, err)
			if strings.HasSuffix(name, ".onnx") {
				require.True(t, bytes.HasPrefix(cached, original))
				require.Equal(t, ocrRuntimeMetadata(), cached[len(original):])
			} else {
				require.Equal(t, original, cached)
			}
		}
		current, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, original, current)
	}
}

func TestOCRRuntimeMetadataContainsOnlyExpectedSessionOptions(t *testing.T) {
	metadata := ocrRuntimeMetadata()
	require.Equal(t, byte(14<<3|2), metadata[0])
	length, consumed := binary.Uvarint(metadata[1:])
	require.Positive(t, consumed)
	entry := metadata[1+consumed:]
	require.Equal(t, int(length), len(entry))
	require.Equal(t, byte(1<<3|2), entry[0])
	keyLength, consumed := binary.Uvarint(entry[1:])
	require.Positive(t, consumed)
	key := entry[1+consumed : 1+consumed+int(keyLength)]
	require.Equal(t, "ort_config", string(key))
	value := entry[1+consumed+int(keyLength):]
	require.Equal(t, byte(2<<3|2), value[0])
	valueLength, consumed := binary.Uvarint(value[1:])
	require.Positive(t, consumed)
	require.Equal(t, int(valueLength), len(value)-1-consumed)
	var options map[string]map[string]int
	require.NoError(t, json.Unmarshal(value[1+consumed:], &options))
	require.Equal(t, map[string]map[string]int{"session_options": {
		"intra_op_num_threads": 2, "inter_op_num_threads": 1,
		"execution_mode": 0, "graph_optimization_level": 99, "enable_profiling": 0,
	}}, options)
}

func TestOCRRuntimeProfileReusesCompleteCacheWithoutRewriting(t *testing.T) {
	source, profile, _ := runtimeProfileFixture(t)
	cache := t.TempDir()
	first := runtimeProfileCommand(source)
	applied, err := applyOCRRuntimeProfile(first, cache, profile)
	require.NoError(t, err)
	require.True(t, applied)
	models := runtimeProfileModelsPath(t, first)
	marker := filepath.Join(models, "det.onnx")
	oldTime := time.Unix(1_000_000, 0)
	require.NoError(t, os.Chtimes(marker, oldTime, oldTime))
	second := runtimeProfileCommand(source)
	applied, err = applyOCRRuntimeProfile(second, cache, profile)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, models, runtimeProfileModelsPath(t, second))
	info, err := os.Stat(marker)
	require.NoError(t, err)
	require.Equal(t, oldTime.Unix(), info.ModTime().Unix())
	before := append([]string(nil), second.Args...)
	applied, err = applyOCRRuntimeProfile(second, cache, profile)
	require.NoError(t, err)
	require.False(t, applied, "an already configured command must not append another model flag")
	require.Equal(t, before, second.Args)
}

func TestOCRRuntimeProfilePublishesOneCompleteCacheConcurrently(t *testing.T) {
	source, profile, _ := runtimeProfileFixture(t)
	cache := t.TempDir()
	const workers = 8
	commands := make([]*exec.Cmd, workers)
	errors := make([]error, workers)
	applied := make([]bool, workers)
	var group sync.WaitGroup
	for i := range commands {
		commands[i] = runtimeProfileCommand(source)
		group.Add(1)
		go func(index int) {
			defer group.Done()
			applied[index], errors[index] = applyOCRRuntimeProfile(commands[index], cache, profile)
		}(i)
	}
	group.Wait()
	var models string
	for i, cmd := range commands {
		require.NoError(t, errors[i])
		require.True(t, applied[i])
		path := runtimeProfileModelsPath(t, cmd)
		if models == "" {
			models = path
		}
		require.Equal(t, models, path)
	}
	entries, err := os.ReadDir(filepath.Dir(models))
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the published cache remains; each caller removes its own staging files")
}

func TestOCRRuntimeProfileSkipsUnknownAssetsAndCustomOptions(t *testing.T) {
	for _, change := range []string{"binary", "model", "dictionary", "custom-models", "custom-rec", "custom-runtime"} {
		t.Run(change, func(t *testing.T) {
			source, profile, assets := runtimeProfileFixture(t)
			cmd := runtimeProfileCommand(source)
			cmd.Env = []string{"CUSTOM=keep"}
			switch change {
			case "binary", "model", "dictionary":
				name := map[string]string{"binary": "RapidOCR-json.exe", "model": "det.onnx", "dictionary": "keys.txt"}[change]
				path := filepath.Join(source, "models", name)
				if change == "binary" {
					path = filepath.Join(source, name)
				}
				data := append([]byte(nil), assets[name]...)
				data[0] ^= 1
				require.NoError(t, os.WriteFile(path, data, 0600))
			case "custom-models":
				cmd.Args = append(cmd.Args, "--models=somewhere-else")
			case "custom-rec":
				cmd.Args = append(cmd.Args, "--rec=custom.onnx")
			case "custom-runtime":
				cmd.Args = append(cmd.Args, "--gpu=1")
			}
			beforeArgs, beforeEnv := append([]string(nil), cmd.Args...), append([]string(nil), cmd.Env...)
			beforeDir := cmd.Dir
			cache := t.TempDir()
			applied, _ := applyOCRRuntimeProfile(cmd, cache, profile)
			require.False(t, applied)
			require.Equal(t, beforeArgs, cmd.Args)
			require.Equal(t, beforeEnv, cmd.Env)
			require.Equal(t, beforeDir, cmd.Dir)
			_, err := os.Stat(filepath.Join(cache, "snapTrans"))
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestOCRRuntimeProfileCorruptCacheFallsBackWithoutReplacingFiles(t *testing.T) {
	for _, corruption := range []string{"same-size", "truncated"} {
		t.Run(corruption, func(t *testing.T) {
			source, profile, _ := runtimeProfileFixture(t)
			cache := t.TempDir()
			first := runtimeProfileCommand(source)
			applied, err := applyOCRRuntimeProfile(first, cache, profile)
			require.NoError(t, err)
			require.True(t, applied)
			models := runtimeProfileModelsPath(t, first)
			path := filepath.Join(models, "rec.onnx")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			if corruption == "same-size" {
				data[0] ^= 1
			} else {
				data = data[:len(data)-1]
			}
			require.NoError(t, os.WriteFile(path, data, 0600))
			unknown := filepath.Join(models, "keep-user-file.txt")
			require.NoError(t, os.WriteFile(unknown, []byte("keep"), 0600))
			second := runtimeProfileCommand(source)
			before := append([]string(nil), second.Args...)
			applied, err = applyOCRRuntimeProfile(second, cache, profile)
			require.ErrorIs(t, err, errOCRRuntimeCache)
			require.False(t, applied)
			require.Equal(t, before, second.Args)
			require.Nil(t, second.Env)
			require.Equal(t, source, second.Dir)
			unchanged, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, data, unchanged)
			unchanged, err = os.ReadFile(unknown)
			require.NoError(t, err)
			require.Equal(t, "keep", string(unchanged))
		})
	}
}

func TestOCRRuntimeProfileCacheFailureLeavesCommandAndParentEnvironmentAlone(t *testing.T) {
	source, profile, _ := runtimeProfileFixture(t)
	t.Setenv("ORT_LOAD_CONFIG_FROM_MODEL", "0")
	cmd := runtimeProfileCommand(source)
	before := append([]string(nil), cmd.Args...)
	blocked := filepath.Join(t.TempDir(), "a-file")
	require.NoError(t, os.WriteFile(blocked, []byte("preserve"), 0600))
	applied, err := applyOCRRuntimeProfile(cmd, blocked, profile)
	require.Error(t, err)
	require.False(t, applied)
	require.Equal(t, before, cmd.Args)
	require.Nil(t, cmd.Env)
	require.Equal(t, source, cmd.Dir)
	require.Equal(t, "0", os.Getenv("ORT_LOAD_CONFIG_FROM_MODEL"))
	applied, err = applyOCRRuntimeProfile(cmd, t.TempDir(), profile)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, "0", os.Getenv("ORT_LOAD_CONFIG_FROM_MODEL"))
	count := 0
	for _, entry := range cmd.Env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "ORT_LOAD_CONFIG_FROM_MODEL") {
			count++
			require.Equal(t, "1", value)
		}
	}
	require.Equal(t, 1, count)
}

func TestOCRRuntimeProfileUsesASCIIModelArgumentWithUnicodeCachePath(t *testing.T) {
	source, profile, _ := runtimeProfileFixture(t)
	cache := filepath.Join(t.TempDir(), "中文 用户", "模型 缓存")
	cmd := runtimeProfileCommand(source)
	applied, err := applyOCRRuntimeProfile(cmd, cache, profile)
	require.NoError(t, err)
	require.True(t, applied)
	require.True(t, strings.HasPrefix(runtimeProfileModelsPath(t, cmd), cache))
	require.Equal(t, "--models=.", cmd.Args[len(cmd.Args)-1])
	require.Equal(t, filepath.Join(source, profile.executable.name), cmd.Path)
}

func TestOCRRuntimeStagingFailureRemovesOnlyOwnFiles(t *testing.T) {
	source, profile, assets := runtimeProfileFixture(t)
	parent := t.TempDir()
	foreign := filepath.Join(parent, ".stage-someone-else", "keep.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(foreign), 0700))
	require.NoError(t, os.WriteFile(foreign, []byte("preserve"), 0600))
	changed := append([]byte(nil), assets["det.onnx"]...)
	changed[0] ^= 1
	require.NoError(t, os.WriteFile(filepath.Join(source, "models", "det.onnx"), changed, 0600))
	destination := filepath.Join(parent, "profile")
	err := stageOCRRuntimeModels(filepath.Join(source, "models"), parent, destination, profile.files, profile.files, ocrRuntimeMetadata())
	require.ErrorIs(t, err, errOCRRuntimeUnsupported, "copying must verify the source again to reject a model changed after eligibility checks")
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, ".stage-someone-else", entries[0].Name())
	data, err := os.ReadFile(foreign)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(data))
}
