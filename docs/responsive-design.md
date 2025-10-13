# Responsive Design

**Status**: Future Enhancement / Community Contribution

This document outlines responsive design improvements to make TapeDeck work well on mobile devices, tablets, and various screen sizes.

## Current State

TapeDeck currently has minimal responsive design:
- Fixed-width layouts (max-width: 1200px, 800px)
- Desktop-focused UI
- Small touch targets
- No mobile navigation patterns

## Goals

- Work on phones (320px+)
- Tablet-optimized layouts (768px-1024px)
- Desktop experience (1024px+)
- Touch-friendly interface
- Mobile-first approach

## Breakpoints

Standard breakpoints to support:

```css
/* Mobile first - base styles apply to smallest screens */

/* Small phones */
@media (min-width: 375px) { }

/* Large phones / small tablets */
@media (min-width: 640px) { }

/* Tablets */
@media (min-width: 768px) { }

/* Desktop */
@media (min-width: 1024px) { }

/* Large desktop */
@media (min-width: 1280px) { }
```

## Components

### Navigation

**Mobile**:
- Hamburger menu icon
- Slide-out drawer
- Bottom navigation bar option
- Larger touch targets (44px minimum)

**Tablet**:
- Collapsible sidebar
- Icon + text labels

**Desktop**:
- Full navigation with text

### Libraries Grid

**Mobile (1 column)**:
```css
.library-grid {
  grid-template-columns: 1fr;
}
```

**Tablet (2 columns)**:
```css
@media (min-width: 768px) {
  .library-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
```

**Desktop (3-4 columns)**:
```css
@media (min-width: 1024px) {
  .library-grid {
    grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  }
}
```

### Card Mappings Dashboard

**Mobile**:
- Stack cards vertically
- Full-width buttons
- Swipe actions for edit/delete
- Bottom sheet for actions

**Tablet/Desktop**:
- Table layout with columns
- Inline edit/delete buttons
- Hover states

### Pairing Mode

**Mobile**:
- Full-screen mode
- Larger "Start Pairing" button
- Bottom sheet for search results
- Larger status indicators
- Sticky controls at bottom

**Desktop**:
- Current centered layout
- Modal for confirmation

### Search

**Mobile**:
- Full-width search input
- Search results overlay entire screen
- Large touch targets for results
- Close button in top corner

**Desktop**:
- Autocomplete dropdown
- Inline results

## Touch Targets

All interactive elements should be at least 44x44px on mobile:

```css
@media (max-width: 768px) {
  button,
  a,
  input[type="checkbox"],
  .clickable {
    min-height: 44px;
    min-width: 44px;
  }
}
```

## Typography

Scale font sizes responsively:

```css
/* Mobile */
body {
  font-size: 16px;
}

h1 {
  font-size: 24px;
}

/* Tablet */
@media (min-width: 768px) {
  body {
    font-size: 18px;
  }

  h1 {
    font-size: 32px;
  }
}

/* Desktop */
@media (min-width: 1024px) {
  h1 {
    font-size: 40px;
  }
}
```

## Spacing

Use responsive spacing:

```css
.container {
  padding: 16px;
}

@media (min-width: 768px) {
  .container {
    padding: 24px;
  }
}

@media (min-width: 1024px) {
  .container {
    padding: 32px;
  }
}
```

## Forms

**Mobile**:
- Full-width inputs
- Large touch-friendly inputs (48px height)
- Bottom sheet for select dropdowns
- Native date/time pickers

**Desktop**:
- Fixed-width forms
- Inline labels
- Hover states

## Images/Media

Use responsive images:

```html
<picture>
  <source media="(min-width: 1024px)" srcset="poster-large.jpg">
  <source media="(min-width: 768px)" srcset="poster-medium.jpg">
  <img src="poster-small.jpg" alt="Movie poster">
</picture>
```

Or CSS:
```css
.media-thumbnail {
  width: 100%;
  height: auto;
  max-width: 200px;
}

@media (min-width: 768px) {
  .media-thumbnail {
    max-width: 250px;
  }
}
```

## Mobile-Specific Features

### Pull-to-Refresh
- Refresh libraries list
- Refresh mappings dashboard

### Swipe Gestures
- Swipe left on mapping to delete
- Swipe right to edit

### Bottom Navigation
- Home
- Libraries
- Mappings
- Settings

### Touch Feedback
```css
button:active {
  transform: scale(0.95);
  background: #cc8f0a;
}
```

## Testing

### Devices to Test
- iPhone SE (375px)
- iPhone 14 (390px)
- iPhone 14 Pro Max (430px)
- iPad Mini (768px)
- iPad Pro (1024px)
- Android phones (various)

### Browser DevTools
- Chrome DevTools device emulation
- Responsive design mode
- Touch simulation

### Real Device Testing
- Use ngrok or similar to test on actual devices
- Test touch interactions
- Test network throttling (3G/4G)

## Implementation Strategy

### Phase 1: Mobile Layouts
1. Add viewport meta tag
2. Make existing components stack on mobile
3. Increase touch target sizes
4. Test on real devices

### Phase 2: Tablet Optimization
1. Add 2-column layouts
2. Optimize for landscape/portrait
3. Better use of screen space

### Phase 3: Enhanced Mobile Features
1. Add swipe gestures
2. Implement bottom navigation
3. Add pull-to-refresh

### Phase 4: Polish
1. Smooth transitions
2. Loading states
3. Offline support (PWA)

## Meta Tags

Add to all pages:

```html
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="black">
```

## CSS Framework Option

Consider using a lightweight CSS framework:
- **Tailwind CSS**: Utility-first, responsive by default
- **Bulma**: Pure CSS, no JS
- **Pico.css**: Minimal, semantic

Or continue with custom CSS using modern techniques:
- CSS Grid
- Flexbox
- Container queries (new!)

## Progressive Web App (PWA)

Consider making TapeDeck installable:
- Add manifest.json
- Service worker for offline support
- App-like experience on mobile
- Home screen icon

## Resources

- [Responsive Design Patterns](https://web.dev/patterns/layout/)
- [Touch Target Sizes](https://web.dev/accessible-tap-targets/)
- [Mobile UX Guidelines](https://www.nngroup.com/articles/mobile-ux/)
- [CSS Grid Guide](https://css-tricks.com/snippets/css/complete-guide-grid/)

## Community Contributions

Responsive design is great for community contributions:
- Mobile layout implementations
- Component refactoring
- Touch gesture support
- PWA features
- Testing on various devices

Good first issues:
1. Add viewport meta tag
2. Make navigation responsive
3. Stack form inputs on mobile
4. Increase button sizes on mobile
5. Add breakpoint for tablets
