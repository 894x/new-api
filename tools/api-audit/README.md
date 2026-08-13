# API Audit Runner

`api-audit` is a repository-owned compatibility and admission-audit CLI. It runs without starting the new-api server and writes a standalone HTML report, a JSON report, and redacted raw HTTP artifacts.

## Build and list cases

```powershell
go build -o .\bin\api-audit.exe .\cmd\api-audit
.\bin\api-audit.exe list --suite openai-chat
.\bin\api-audit.exe list --suite seedance
```

OpenAI Chat contains 43 built-in case directories corresponding to the supplied T1-T43 audit dimensions. Eighteen cases currently make an automated protocol request. Twenty-five cases explicitly return `unknown` because a trustworthy result requires a model-specific tokenizer, official fingerprint, historical baseline, repeated/statistical execution, timing instrumentation, billing-log access, or manual semantic review. They remain first-class cases in the report instead of being silently skipped or falsely marked as passing.

## Run OpenAI Chat audits

The key is read from an environment variable and is never accepted as a normal command argument:

```powershell
$env:API_AUDIT_API_KEY = "<secret>"

# Safe smoke test: only the default synchronous-response case
.\bin\api-audit.exe run `
  --suite openai-chat `
  --base-url "https://gateway.example" `
  --model "model-name"

# Full T1-T43 admission report
.\bin\api-audit.exe run `
  --suite openai-chat `
  --base-url "https://gateway.example" `
  --model "model-name" `
  --all-cases `
  --output ".\output\api-audit\chat-full"

# One or more selected cases
.\bin\api-audit.exe run `
  --suite openai-chat `
  --base-url "https://gateway.example" `
  --model "model-name" `
  --case T004 --case T009
```

Use `--dry-run` to render requests and reports without network calls.

## Run Seedance audits

The default remains one text-only `doubao-seedance-2-0-260128` task at 480p for 4 seconds:

```powershell
.\bin\api-audit.exe run `
  --suite seedance `
  --base-url "https://gateway.example"
```

The full matrix is six case folders multiplied by three configured models. A live 18-task run is rejected unless the paid-suite confirmation flag is present:

```powershell
# Inspect all 18 payloads without submitting
.\bin\api-audit.exe run --suite seedance --base-url "https://gateway.example" --all-cases --all-models --dry-run

# Submit all 18 tasks
.\bin\api-audit.exe run --suite seedance --base-url "https://gateway.example" --all-cases --all-models --confirm-paid-suite
```

The five multimodal Seedance cases use the public image/video inputs from the Volcengine video-generation tutorial. Task creation is not treated as final success; the runner polls to terminal status unless `--no-wait` is supplied.

## Case folders

Every test owns one directory and one `case.json`:

```text
tools/api-audit/cases/
├── openai-chat/
│   ├── T001-sync-response/case.json
│   └── ... 43 total
└── seedance/
    ├── V001-text/case.json
    └── ... 6 total
```

The file contains the stable ID, display name, dimension, protocol, runner kind, default-selection flag, severity, HTTP request, and case-specific constraints. Adding or changing a case does not require editing a global manifest.

## Report contract

Each run writes:

```text
<output>/
├── report.html
├── report.json
└── raw/<case-id>/exchange-*.json
```

The report includes the overall verdict, pass/warning/fail/unknown counts, dimension totals, critical failures, warnings, timings, evidence, usage, model, and raw exchanges. Bearer credentials are never captured. Query strings are stripped from URLs so signed TOS credentials are not persisted.

`pkg/apiaudit` is the reusable integration boundary for a later administrator-only backend workflow; `cmd/api-audit` contains only CLI parsing and orchestration.
