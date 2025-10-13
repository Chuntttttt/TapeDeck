# Accessibility Enhancements

**Status**: Future Enhancement / Community Contribution

This document outlines accessibility improvements for TapeDeck to make it usable for people with disabilities.

## Goals

- Full keyboard navigation support
- Screen reader compatibility
- WCAG 2.1 AA compliance
- High contrast mode support

## Keyboard Navigation

### Global
- Tab order should follow logical flow
- Escape key closes modals/dropdowns
- Enter/Space activates buttons
- Arrow keys navigate lists

### Specific Components

**Search Autocomplete**:
- Arrow up/down to navigate results
- Enter to select
- Escape to close
- Tab moves to next focusable element

**Mappings Dashboard**:
- Tab through card list
- Enter/Space to edit or delete
- Focus management on delete confirmation

**Pairing Mode**:
- Tab through search → "Start Pairing" → status indicators
- Enter on search result selects it
- Visual focus indicators at all times

## Screen Reader Support

### ARIA Labels

All interactive elements need proper labels:
```html
<button aria-label="Search media">
  <svg>...</svg>
</button>

<div role="status" aria-live="polite">
  Connected to Home Assistant
</div>

<div role="alert" aria-live="assertive">
  Error: Tag already mapped
</div>
```

### Semantic HTML

Use proper HTML5 semantic elements:
- `<nav>` for navigation
- `<main>` for main content
- `<aside>` for sidebars
- `<article>` for card items
- `<button>` instead of clickable divs

### Form Labels

All inputs need associated labels:
```html
<label for="tag-id">NFC Tag ID</label>
<input id="tag-id" type="text" name="tag_id">
```

### Status Announcements

Important state changes should be announced:
- "Connecting to Home Assistant"
- "Tag detected: 04-16-5C-D4-2E-61-80"
- "Mapping created successfully"
- "Error: Connection failed"

## Color Contrast

### Minimum Ratios (WCAG AA)
- Normal text: 4.5:1
- Large text (18pt+): 3:1
- UI components: 3:1

### Current Palette Audit Needed
- Check all text/background combinations
- Verify button states (hover, focus, disabled)
- Test error messages (red on white)
- Review status indicators (green/yellow/red)

## High Contrast Mode

Support Windows High Contrast Mode and similar:
- Don't rely solely on color for information
- Ensure borders are visible in high contrast
- Icons should have text alternatives
- Test in forced-colors media query

```css
@media (forced-colors: active) {
  .status-indicator {
    border: 2px solid ButtonText;
  }
}
```

## Focus Indicators

All interactive elements need visible focus:
```css
button:focus-visible,
input:focus-visible,
a:focus-visible {
  outline: 2px solid #e5a00d;
  outline-offset: 2px;
}
```

Never use `outline: none` without providing an alternative.

## Implementation Checklist

- [ ] Audit all pages with keyboard-only navigation
- [ ] Test with NVDA/JAWS screen readers
- [ ] Run axe or Lighthouse accessibility audit
- [ ] Add ARIA labels to all interactive elements
- [ ] Implement skip links ("Skip to main content")
- [ ] Add focus trap for modals
- [ ] Test with Windows High Contrast Mode
- [ ] Document keyboard shortcuts
- [ ] Add alt text to all images/icons
- [ ] Ensure form validation errors are announced
- [ ] Test color contrast ratios
- [ ] Add loading states with aria-busy

## Testing Tools

- **axe DevTools**: Browser extension for automated testing
- **Lighthouse**: Built into Chrome DevTools
- **NVDA**: Free screen reader for Windows
- **VoiceOver**: Built into macOS
- **Keyboard**: Unplug mouse and navigate site

## Resources

- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)
- [MDN Accessibility](https://developer.mozilla.org/en-US/docs/Web/Accessibility)
- [WebAIM](https://webaim.org/)
- [A11y Project Checklist](https://www.a11yproject.com/checklist/)

## Community Contributions

This is an excellent area for community contributions. Contributors can:
- Run accessibility audits and file issues
- Implement keyboard navigation
- Add ARIA labels
- Write accessibility tests
- Improve documentation

Good first issues for accessibility:
1. Add proper heading hierarchy (h1-h6)
2. Implement focus indicators
3. Add alt text to icons
4. Create skip links
5. Add form label associations
