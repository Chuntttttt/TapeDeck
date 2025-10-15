# Security Audit & Recommendations

**Last Updated**: 2025-01-15

This document outlines security issues found in the TapeDeck codebase and provides actionable remediation steps.

## Completed ✅

- **CRIT-1**: Session Cookies - Implemented `REQUIRE_TLS` config with startup warnings
- **CRIT-2**: WebSocket Origins - Implemented origin checking with dev mode localhost exemption
- **CRIT-3**: Default Session Secret - Auto-generated and persisted in `.session_secret`
- **CRIT-4**: CSRF Protection - Implemented with gorilla/csrf middleware
- **HIGH-1**: Token Encryption - AES-256-GCM encryption for Plex and HA tokens

## Not Applicable / Infrastructure Layer

- **CRIT-5**: Rate Limiting - Self-hosted local app; handle at firewall/reverse proxy if exposed publicly
- **HIGH-2**: Session Duration - Set to 10 years (effectively forever) for single-user convenience. Plex tokens don't expire, so forced re-auth is pointless.
- **HIGH-3**: Unauthenticated Metrics - Intentionally public for Prometheus scraping

## Remaining Recommendations

### 🟡 Input Validation (Optional)

**Status**: Low priority for self-hosted single-user app. SQL injection protected by parameterized queries.

**User Inputs**:
- `ha_url`: Home Assistant URL (setup/settings)
- `apple_tv_entity`: HA entity ID like "media_player.apple_tv" (mappings)
- `tag_id`, `media_id`, `media_title`: Card mapping data (user-controlled, low risk)

**Minimal useful validation** (if desired):
- URL format validation for `ha_url` (prevent typos)
- Entity ID format check for `apple_tv_entity` (ensure contains dot)
- String length limits (prevent absurdly long inputs)

Most validation not necessary since:
- Single trusted user (yourself)
- Parameterized queries prevent SQL injection
- Bad data just causes errors, not security issues

**Timeline**: 1-2 hours if implemented

---

### 🟡 Session Fixation (Low Priority)

**Status**: Low risk for single-user self-hosted app. Could rotate session ID after login.

**Timeline**: 30 minutes if implemented

---

### 🟡 Audit Logging (Optional)

**Status**: Already have structured logging. Could add more detailed logging for sensitive operations.

**Timeline**: 1 hour if implemented

---

## Security Checklist for Deployment

- [x] SESSION_SECRET auto-generated and persisted in `.session_secret`
- [x] CSRF_KEY auto-generated and persisted in `.csrf_key`
- [x] ENCRYPTION_KEY auto-generated and persisted in `.encryption_key`
- [x] Session cookies use Secure flag when REQUIRE_TLS=true
- [x] WebSocket origin checking implemented (dev mode allows localhost)
- [x] CSRF protection on all HTML forms
- [x] Plex and HA tokens encrypted at rest with AES-256-GCM
- [ ] REQUIRE_TLS=true if exposed to internet
- [ ] DevMode disabled (DEV_MODE=false) in production
- [ ] Firewall rules restrict access if publicly exposed
- [ ] Regular backups of database and config files
