# Open Source Compliance

This document records the compliance posture for the SunAPI open source tree.
It is an engineering checklist, not legal advice.

## Included Source

Only these source trees are included:

- `front/`
- `backend/`

Excluded by design:

- `front-new/`
- `new-api/`
- generated frontend bundles such as `dist/`
- dependency folders such as `node_modules/`
- local caches such as `.cache/`, `.tmp/`, `.tanstack/`
- generated backend static bundles such as `backend/static/management.html`
- built binaries such as `sunapi.exe`

The release keeps frontend UI code, charts, icons, motion code, and public
assets that live under `front/`. It does not import experimental assets from
`front-new/`.

## AGPLv3 Obligations

The frontend includes files derived from QuantumNous/new-api and marked with
GNU AGPLv3 notices. For AGPL-covered code:

- Preserve copyright and license headers.
- Preserve `front/LICENSE`.
- Preserve `NOTICE` attribution.
- Preserve `THIRD-PARTY-LICENSES.md` where it applies to frontend dependencies
  and upstream notices.
- Provide complete corresponding source code for modified network services.
- Keep a visible source code link in the web UI or legal/about page.
- Mark local modifications clearly.
- Do not use technical protection measures to prevent users from exercising
  rights granted by the AGPL.

## MIT Obligations

The backend is MIT licensed. Preserve:

- `backend/LICENSE`
- copyright notices
- third-party notices where applicable

## Release Checklist

Before publishing a release:

- [ ] Run frontend build and type checks.
- [ ] Run backend tests.
- [ ] Verify `front/dist`, `front/node_modules`, backend binaries, and caches are
      not committed into this open source tree.
- [ ] Verify `NOTICE` is included in archives, Docker images, and installers.
- [ ] Verify the UI exposes source and attribution links where AGPL code is used.
- [ ] Generate checksums for release archives and binaries.
- [ ] Sign release artifacts when possible.
- [ ] Publish the exact corresponding source for every public network service
      version.

## Suggested UI Legal Links

At minimum, expose:

- Source code
- License
- Notices
- Security policy
- Original new-api project link
