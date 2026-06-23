# Release Process

## Prepare

1. Verify the open source tree contains only `front/`, `backend/`, and release
   metadata.
2. Run frontend checks.
3. Run backend tests.
4. Review `OPEN_SOURCE_COMPLIANCE.md`.
5. Review `backend/config.example.yaml` before enabling management panel
   downloads or auto-updates.

## Build

Frontend:

```powershell
cd front
npm install
npm run build
```

Backend:

```powershell
cd backend
go test ./...
go build ./cmd/server
```

## Publish

For each release artifact:

- publish the exact corresponding source archive
- include `NOTICE`, `OPEN_SOURCE_COMPLIANCE.md`, `SECURITY.md`, and
  `TRADEMARKS.md`
- include `LICENSE`, `front/LICENSE`, `backend/LICENSE`, and
  `THIRD-PARTY-LICENSES.md`
- include checksums
- sign artifacts when possible

For hosted services using AGPL-covered code:

- publish the corresponding source for the deployed version
- provide a visible source link in the UI
- preserve upstream attribution
