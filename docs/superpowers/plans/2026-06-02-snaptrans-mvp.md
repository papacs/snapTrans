# snapTrans MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Wails v2 + Vue 3 MVP skeleton for instant screenshot translation and document the remaining engineering details.

**Architecture:** Go owns config, capture, OCR, hotkey registration, and DeepSeek streaming. Vue owns the frozen-screen interaction, DPI-safe crop generation, settings UI, and streamed result rendering. A frontend browser fallback makes UI work testable before Wails is installed.

**Tech Stack:** Wails v2.12.0, Go 1.21+, Vue 3.5, Vite 8, TypeScript 6, Tailwind CSS 3.4, Vitest, RapidOCR, DeepSeek.

---

### Task 1: Project Documentation

**Files:**
- Create: `.gitignore`
- Create: `.env.sample`
- Create: `AGENTS.md`
- Create: `README.md`
- Create: `docs/TODO.md`
- Create: `docs/superpowers/specs/2026-06-02-snaptrans-mvp-design.md`

- [x] Write project setup, environment, verification, and TODO docs.
- [x] Keep `.env`, OCR binaries, build output, node modules, and generated Wails JS out of git.

### Task 2: Frontend Tests First

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/vitest.config.ts`
- Create: `frontend/src/utils/selection.test.ts`

- [x] Write tests for rectangle normalization and CSS-to-image coordinate mapping.
- [x] Run `cd frontend && npm install`.
- [x] Run `cd frontend && npm test` and confirm the tests fail because `selection.ts` has not been implemented.

### Task 3: Frontend Implementation

**Files:**
- Create: `frontend/index.html`
- Create: `frontend/src/main.ts`
- Create: `frontend/src/App.vue`
- Create: `frontend/src/services/backend.ts`
- Create: `frontend/src/utils/selection.ts`
- Create: `frontend/src/styles.css`
- Create: `frontend/src/types/wails.d.ts`

- [x] Implement selection utilities to pass the failing tests.
- [x] Implement backend bridge with Wails global calls and browser fallback.
- [x] Implement full-screen canvas overlay, instant mouseup processing, streaming result box, settings modal, copy, and restore.
- [x] Run frontend tests, typecheck, and build.

### Task 4: Wails Backend Skeleton

**Files:**
- Create: `go.mod`
- Create: `wails.json`
- Create: `main.go`
- Create: `app.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/capture/capture.go`
- Create: `internal/hotkeys/hotkeys.go`
- Create: `internal/ocr/rapidocr.go`
- Create: `internal/ocr/rapidocr_test.go`
- Create: `internal/translator/deepseek.go`

- [x] Implement local config and `.env` fallback.
- [x] Implement all-monitor screenshot capture as a PNG data URL.
- [x] Implement shortcut parsing and registration for common combinations.
- [x] Implement RapidOCR execution and flexible JSON text extraction.
- [x] Implement DeepSeek streaming token callback.
- [x] Implement Wails-bound `LoadConfig`, `SaveConfig`, `TriggerCapture`, `ProcessImage`, and `HideWindow`.
- [x] Document that Go tests cannot run until Go is installed.

### Task 5: Verification

**Files:**
- Verify: frontend and repo root

- [x] Run `cd frontend && npm test`.
- [x] Run `cd frontend && npm run typecheck`.
- [x] Run `cd frontend && npm run build`.
- [x] Run `go test ./...` after installing Go.
- [x] Run `wails version` after installing Wails.
- [x] Run `wails build`.
