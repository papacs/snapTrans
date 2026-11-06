package ocr

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ORT 1.12.1 reads these session options from the ort_config model metadata:
// https://github.com/microsoft/onnxruntime/blob/v1.12.1/onnxruntime/core/session/inference_session_utils.cc
const ocrRuntimeConfigJSON = `{"session_options":{"intra_op_num_threads":2,"inter_op_num_threads":1,"execution_mode":0,"graph_optimization_level":99,"enable_profiling":0}}`

type ocrRuntimeAsset struct {
	name, digest string
	size         int64
	model        bool
}

type ocrRuntimeProfile struct {
	executable ocrRuntimeAsset
	files      []ocrRuntimeAsset
}

var stockOCRRuntimeProfile = ocrRuntimeProfile{
	executable: ocrRuntimeAsset{name: "RapidOCR-json.exe", size: 16849920, digest: "616bd43a4672ee9fcf4d1ca607d7d9ec413b45d489482064d97e00a7efa9d76e"},
	files: []ocrRuntimeAsset{
		{name: "ch_PP-OCRv3_det_infer.onnx", size: 2432880, digest: "3439588c030faea393a54515f51e983d8e155b19a2e8aba7891934c1cf0de526", model: true},
		{name: "ch_ppocr_mobile_v2.0_cls_infer.onnx", size: 585532, digest: "e47acedf663230f8863ff1ab0e64dd2d82b838fceb5957146dab185a89d6215c", model: true},
		{name: "ch_PP-OCRv3_rec_infer.onnx", size: 10690752, digest: "897a3ededb38fee0dae2c1ccee38241f37df202c9509e3abca02e9217c5ee615", model: true},
		{name: "ppocr_keys_v1.txt", size: 26249, digest: "28b2362ad4ab2dc38769aa72feb535e3a9ddb3fd2a7585a05920e6393b1dc7f7"},
	},
}

var (
	errOCRRuntimeUnsupported = errors.New("OCR runtime assets are not the supported stock bundle")
	errOCRRuntimeCache       = errors.New("OCR runtime model cache failed validation")
)

// configureOCRRuntime is optional: custom installations and cache failures keep
// the original command. Only this child's environment and cached model copies change.
func configureOCRRuntime(cmd *exec.Cmd) {
	started := time.Now()
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return
	}
	applied, err := applyOCRRuntimeProfile(cmd, cacheRoot, stockOCRRuntimeProfile)
	if errors.Is(err, errOCRRuntimeUnsupported) {
		return
	}
	if err != nil {
		reason := "filesystem"
		if errors.Is(err, os.ErrPermission) {
			reason = "permission"
		} else if errors.Is(err, errOCRRuntimeCache) {
			reason = "cache-validation"
		}
		slog.Warn("OCR runtime profile unavailable", "profile", "cpu-2", "stage_ms", time.Since(started).Milliseconds(), "reason", reason)
	} else if applied {
		slog.Info("OCR runtime profile ready", "profile", "cpu-2", "stage_ms", time.Since(started).Milliseconds())
	}
}

func applyOCRRuntimeProfile(cmd *exec.Cmd, cacheRoot string, profile ocrRuntimeProfile) (bool, error) {
	if cmd == nil || len(cmd.Args) == 0 || !strings.EqualFold(filepath.Base(cmd.Path), profile.executable.name) {
		return false, nil
	}
	// Explicit model/provider/runtime options belong to the user's installation.
	for _, arg := range cmd.Args[1:] {
		if !strings.HasPrefix(arg, "--image=") && !strings.HasPrefix(arg, "--image_path=") && !strings.HasPrefix(arg, "--ensureAscii=") {
			return false, nil
		}
	}
	actual, _, err := hashOCRRuntimeAsset(cmd.Path, profile.executable.size, nil)
	if err != nil || actual != profile.executable.digest {
		return false, errOCRRuntimeUnsupported
	}
	metadata := ocrRuntimeMetadata()
	source := filepath.Join(filepath.Dir(cmd.Path), "models")
	expected := make([]ocrRuntimeAsset, 0, len(profile.files))
	for _, file := range profile.files {
		original, configured, err := hashOCRRuntimeAsset(filepath.Join(source, file.name), file.size, metadata)
		if err != nil || original != file.digest {
			return false, errOCRRuntimeUnsupported
		}
		if file.model {
			file.digest = configured
			file.size += int64(len(metadata))
		}
		expected = append(expected, file)
	}
	digest := sha256.New()
	fmt.Fprintln(digest, ocrRuntimeConfigJSON, profile.executable.digest)
	for _, file := range expected {
		fmt.Fprintln(digest, file.name, file.size, file.digest)
	}
	cacheRoot, err = filepath.Abs(cacheRoot)
	if err != nil {
		return false, err
	}
	parent := filepath.Join(cacheRoot, "snapTrans", "ocr-models")
	destination := filepath.Join(parent, hex.EncodeToString(digest.Sum(nil)))
	if err := stageOCRRuntimeModels(source, parent, destination, profile.files, expected, metadata); err != nil {
		return false, err
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	nextEnv := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "ORT_LOAD_CONFIG_FROM_MODEL") {
			nextEnv = append(nextEnv, entry)
		}
	}
	// Go passes the working directory to Windows as UTF-16. Keep model argv
	// ASCII because this stock executable's narrow argv can lose Unicode paths.
	cmd.Dir = destination
	cmd.Args = append(cmd.Args, "--models=.")
	cmd.Env = append(nextEnv, "ORT_LOAD_CONFIG_FROM_MODEL=1")
	return true, nil
}

// Sum the original and configured bytes in one pass. Reading each model again
// only to append metadata delays OCR readiness on cold or busy disks.
func hashOCRRuntimeAsset(path string, size int64, suffix []byte) (original, configured string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return "", "", errOCRRuntimeCache
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", "", err
	}
	original = hex.EncodeToString(digest.Sum(nil))
	_, _ = digest.Write(suffix)
	return original, hex.EncodeToString(digest.Sum(nil)), nil
}

func validateOCRRuntimeCache(directory string, expected []ocrRuntimeAsset) error {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errOCRRuntimeCache
	}
	for _, file := range expected {
		path := filepath.Join(directory, file.name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return errOCRRuntimeCache
		}
		digest, _, err := hashOCRRuntimeAsset(path, file.size, nil)
		if err != nil || digest != file.digest {
			return errOCRRuntimeCache
		}
	}
	return nil
}

func stageOCRRuntimeModels(source, parent, destination string, original, expected []ocrRuntimeAsset, metadata []byte) error {
	if _, err := os.Lstat(destination); err == nil {
		return validateOCRRuntimeCache(destination, expected)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".stage-")
	if err != nil {
		return err
	}
	defer func() {
		// Remove only files in this invocation's staging directory, never an
		// existing cache or any original model directory.
		for _, file := range original {
			_ = os.Remove(filepath.Join(stage, file.name))
		}
		_ = os.Remove(stage)
	}()
	for _, file := range original {
		if err := copyOCRRuntimeAsset(filepath.Join(source, file.name), filepath.Join(stage, file.name), file, metadata); err != nil {
			return err
		}
	}
	if err := validateOCRRuntimeCache(stage, expected); err != nil {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		// Another process can atomically publish the same complete cache first.
		if validateOCRRuntimeCache(destination, expected) == nil {
			return nil
		}
		return err
	}
	return nil
}

func copyOCRRuntimeAsset(source, destination string, asset ocrRuntimeAsset, metadata []byte) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	digest := sha256.New()
	copied, err := io.Copy(io.MultiWriter(output, digest), input)
	if err == nil && (copied != asset.size || hex.EncodeToString(digest.Sum(nil)) != asset.digest) {
		err = errOCRRuntimeUnsupported
	}
	if err == nil && asset.model {
		_, err = output.Write(metadata)
	}
	closeErr := output.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// ModelProto.metadata_props is field 14, containing StringStringEntryProto
// fields 1 (key) and 2 (value). Append only; graph and weight bytes stay intact.
// https://github.com/onnx/onnx/blob/v1.12.0/onnx/onnx.proto
func ocrRuntimeMetadata() []byte {
	entry := append([]byte{0x0a, 10}, "ort_config"...)
	entry = append(entry, 0x12)
	entry = binary.AppendUvarint(entry, uint64(len(ocrRuntimeConfigJSON)))
	entry = append(entry, ocrRuntimeConfigJSON...)
	field := binary.AppendUvarint([]byte{0x72}, uint64(len(entry)))
	return append(field, entry...)
}
