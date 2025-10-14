# Templ Migration Guide

This guide shows how to migrate handlers from inline HTML strings to Templ templates.

## Migration Complete

Successfully migrated the setup welcome page as a proof of concept. The pattern is established and can be replicated for other handlers.

## What Was Done

### 1. Created Base Infrastructure

**Layouts** (`templates/layouts/`):
- `base.templ` - Common HTML structure for all pages (DOCTYPE, head, links to CSS)
- `setup.templ` - Setup wizard layout with progress bar

**Components** (`templates/components/`):
- `buttons.templ` - Reusable button components (Button, ButtonLink, SecondaryButton, BackLink)
- `status.templ` - Status message components (StatusBox, ErrorBox, InfoBox)

**Styles** (`static/css/main.css`):
- Extracted common CSS from all handlers
- Includes: buttons, forms, progress bars, status messages, typography, lists, etc.

**Static File Server** (`main.go`):
- Added `/static/` route to serve CSS, JS, and images

### 2. Migrated Setup Welcome Page

**Before** (inline HTML in `setup.go`):
```go
func (h *SetupHandler) Step1Welcome(w http.ResponseWriter, r *http.Request) {
    // ... save state ...
    _, _ = fmt.Fprintf(w, `<!DOCTYPE html>...</html>`)
}
```

**After** (using Templ):
```go
import "github.com/Chuntttttt/tapedeck/templates/pages"

func (h *SetupHandler) Step1Welcome(w http.ResponseWriter, r *http.Request) {
    // ... save state ...
    if err := pages.SetupWelcome().Render(r.Context(), w); err != nil {
        log.Printf("Failed to render template: %v", err)
        http.Error(w, "Failed to render page", http.StatusInternalServerError)
    }
}
```

**Template** (`templates/pages/setup_welcome.templ`):
```templ
package pages

import "github.com/Chuntttttt/tapedeck/templates/layouts"
import "github.com/Chuntttttt/tapedeck/templates/components"

templ SetupWelcome() {
    @layouts.SetupLayout("Setup", 1, welcomeContent())
}

templ welcomeContent() {
    <div class="container">
        <div class="emoji">🎬</div>
        <h1>Welcome to TapeDeck</h1>
        <!-- content -->
        @components.ButtonLink("Get Started", "/setup/plex", templ.Attributes{})
    </div>
}
```

## Migration Pattern for Remaining Handlers

### Step 1: Create the Templ Template

Create a new file in `templates/pages/` (e.g., `setup_plex_login.templ`):

```templ
package pages

import "github.com/Chuntttttt/tapedeck/templates/layouts"
import "github.com/Chuntttttt/tapedeck/templates/components"

// SetupPlexLogin renders the Plex login page
templ SetupPlexLogin() {
    @layouts.SetupLayout("Connect Plex", 2, plexLoginContent())
}

templ plexLoginContent() {
    <div class="container">
        @components.BackLink("/setup", "Back")
        <h1>Connect Your Plex Account</h1>
        <p>TapeDeck needs access to your Plex servers to browse and play media.</p>
        <p>Click the button below to authenticate with your Plex account.</p>
        @components.ButtonLink("Login with Plex", "/auth/login", templ.Attributes{})
    </div>
}
```

### Step 2: Update the Handler

In `internal/handlers/setup.go`:

```go
// Import the pages package (add to imports at top of file)
import "github.com/Chuntttttt/tapedeck/templates/pages"

// Replace the render function
func (h *SetupHandler) renderPlexLogin(w http.ResponseWriter, r *http.Request) {
    if err := pages.SetupPlexLogin().Render(r.Context(), w); err != nil {
        log.Printf("Failed to render template: %v", err)
        http.Error(w, "Failed to render page", http.StatusInternalServerError)
    }
}
```

### Step 3: Generate Templ Files

```bash
templ generate
```

### Step 4: Test

```bash
go build -o ./tmp/main .
# Or just use air which will auto-rebuild
```

## For Templates with Dynamic Data

When you need to pass data to templates (like the server selection page), add parameters:

```templ
package pages

import "github.com/Chuntttttt/tapedeck/templates/layouts"
import "github.com/Chuntttttt/tapedeck/internal/config"

templ SetupServerSelection(servers []config.PlexServer) {
    @layouts.SetupLayout("Select Plex Servers", 2, serverSelectionContent(servers))
}

templ serverSelectionContent(servers []config.PlexServer) {
    <div class="container">
        <h1>Select Plex Servers</h1>
        <p>We found { fmt.Sprintf("%d", len(servers)) } server(s)...</p>

        <form method="post" action="/setup/plex/save">
            <div class="server-list">
                for _, server := range servers {
                    <label class="server-item">
                        <input type="checkbox" name="server_ids" value={ server.ID } checked/>
                        <div>
                            <div class="server-name">{ server.Name } ({ server.Owner })</div>
                        </div>
                    </label>
                }
            </div>
            <button type="submit" class="btn">Continue</button>
        </form>
    </div>
}
```

Then in the handler:

```go
func (h *SetupHandler) renderServerSelection(w http.ResponseWriter, r *http.Request, servers []config.PlexServer) {
    if err := pages.SetupServerSelection(servers).Render(r.Context(), w); err != nil {
        log.Printf("Failed to render template: %v", err)
        http.Error(w, "Failed to render page", http.StatusInternalServerError)
    }
}
```

## For Templates with JavaScript

Include JavaScript directly in the template using `<script>` tags, or better yet, move it to `static/js/` and include it:

```templ
templ SetupHomeAssistant(haURL string, haToken string) {
    @layouts.SetupLayout("Connect Home Assistant", 3, haContent(haURL, haToken))
    <script src="/static/js/setup-ha.js"></script>
}
```

## Benefits of This Approach

1. **Type Safety**: Templ catches errors at compile time
2. **Code Reuse**: Shared components (buttons, layouts, etc.)
3. **Separation of Concerns**: HTML in templates, logic in handlers
4. **Better IDE Support**: Syntax highlighting for Templ files
5. **No Manual Escaping**: Templ handles HTML escaping automatically
6. **Easier Maintenance**: Changes to common styles only need to happen in one place

## Remaining Handlers to Migrate

### Setup Pages (high priority, lots of duplication):
- `renderPlexLogin` → `templates/pages/setup_plex_login.templ`
- `renderPlexError` → `templates/pages/setup_plex_error.templ`
- `renderServerSelection` → `templates/pages/setup_server_selection.templ`
- `Step3HomeAssistant` → `templates/pages/setup_home_assistant.templ`
- `renderAppleTVSelection` → `templates/pages/setup_appletv_selection.templ`
- `renderAppleTVEmpty` → `templates/pages/setup_appletv_empty.templ`
- `renderAppleTVError` → `templates/pages/setup_appletv_error.templ`
- `Step5Complete` → `templates/pages/setup_complete.templ`

### Media Pages:
- `media.go` - Libraries, LibraryContents, Search

### Mappings Pages:
- `mappings.go` - Dashboard, NewMappingForm, EditMappingForm

### Pairing Page:
- `pairing.go` - PairForm (this one has complex JavaScript, keep JS inline or move to static/js/)

### Auth Pages:
- `auth.go` - Login page

## Tips

1. Start with the simple pages first (welcome, error pages)
2. For pages with forms, use `templ.Attributes{}` to pass additional HTML attributes to components
3. For pages with lots of JavaScript, consider moving JS to `static/js/` for better organization
4. Run `templ generate` after every template change (Air does this automatically)
5. Use `@componentName()` syntax to call other templates
6. Use `{ variableName }` to output variables
7. Templ automatically escapes HTML - no need for `html.EscapeString()`

## Example: Complete Migration of a Simple Page

**Before:**
```go
func (h *SetupHandler) renderPlexError(w http.ResponseWriter, _ *http.Request, errorMsg string) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    _, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Error</title><style>...</style></head>
<body>
    <div class="error">%s</div>
    <a href="/setup/plex" class="btn">Try Again</a>
</body>
</html>`, html.EscapeString(errorMsg))
}
```

**After:**

Template (`templates/pages/setup_plex_error.templ`):
```templ
package pages

import "github.com/Chuntttttt/tapedeck/templates/layouts"
import "github.com/Chuntttttt/tapedeck/templates/components"

templ SetupPlexError(errorMsg string) {
    @layouts.SetupLayout("Plex Error", 2, plexErrorContent(errorMsg))
}

templ plexErrorContent(errorMsg string) {
    <div class="container">
        @components.BackLink("/setup", "Back")
        <h1>❌ Plex Server Error</h1>
        @components.ErrorBox(errorMsg)
        <p>This usually happens when your Plex authentication has expired.</p>
        @components.ButtonLink("Re-authenticate", "/auth/logout?redirect=/setup/plex", templ.Attributes{})
        @components.ButtonLink("Try Again", "/setup/plex", templ.Attributes{"class": "btn-secondary"})
    </div>
}
```

Handler:
```go
func (h *SetupHandler) renderPlexError(w http.ResponseWriter, r *http.Request, errorMsg string) {
    if err := pages.SetupPlexError(errorMsg).Render(r.Context(), w); err != nil {
        log.Printf("Failed to render template: %v", err)
        http.Error(w, "Failed to render page", http.StatusInternalServerError)
    }
}
```

Much cleaner!
