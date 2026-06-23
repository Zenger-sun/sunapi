# SunAPI Open Source Edition

SunAPI Open Source Edition contains only the source trees intended for public
release:

- `front/` - the web console.
- `backend/` - the local proxy server.

Experimental, clean-room, cache, build, and packaging workspaces are intentionally
not included in this tree.

All UI source, icons, charts, animations, and public assets that belong to the
release are kept inside `front/`. Experimental assets from `front-new/` are not
part of this open source edition.

## License Summary

This repository is a license-aware aggregate. Different subdirectories may be
covered by different licenses:

- `backend/` is licensed under the MIT License. See `backend/LICENSE`.
- `front/` includes code derived from the new-api frontend and is distributed
  under the GNU Affero General Public License v3.0 or later where those file
  headers apply. See `front/LICENSE`.
- Files with explicit license headers keep those headers and terms.
- The root `LICENSE` file is a license map, not a single-license override.

When running a modified network service based on AGPL-covered frontend code,
you must provide the complete corresponding source code to users who interact
with the service over a network.

## Upstream Attribution

The frontend contains AGPL-covered work by QuantumNous/new-api contributors.
Preserve the visible attribution required by upstream notices:

> Frontend design and development by New API contributors.

Also preserve a prominent link to the original project:

https://github.com/QuantumNous/new-api

See `NOTICE` and `OPEN_SOURCE_COMPLIANCE.md`.

## Development

### Frontend

```powershell
cd front
npm install
npm run dev
npm run build
```

The open source copy is prepared as a standalone tree. Workspace-only dependency
specifiers are replaced with explicit versions in `front/package.json`.

### Backend

```powershell
cd backend
go test ./...
go build ./cmd/server
```

## Security

Do not treat source availability as a security weakness. Security controls should
be implemented through authentication, authorization, rate limiting, secret
handling, signed releases, and secure defaults.

Report vulnerabilities according to `SECURITY.md`.

## Brand

SunAPI names, logos, and release channels are project brand assets. Open source
license grants apply to code, not to permission to impersonate official SunAPI
releases or services. See `TRADEMARKS.md`.
