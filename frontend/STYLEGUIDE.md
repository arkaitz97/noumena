# Noumena UI Styling Guide

This document outlines the core design system and standard practices for building the frontend of the Noumena SaaS platform. It is intended to help developers and designers maintain the "premium, sophisticated SaaS" aesthetic uniformly across the app.

---

## 1. Core Principles

- **Contrast over Clutter**: Rely on distinct grayscale separations (`zinc-50` backgrounds vs `white` foreground cards with `zinc-900` text) instead of heavy, loud separators.
- **Soft Geometry**: We favor highly rounded styling (`rounded-xl` and `rounded-2xl`) combined with delicate borders (`ring-1 ring-zinc-950/5`) to make elements look floating rather than boxed.
- **Glassmorphism & Depth**: Make use of layered ambient lighting (blurred blobs) behind standard white components, or use `backdrop-blur-sm` for sticky/floating overlays.
- **Alive but Restrained**: Interactivity (hover/focus states) should be ubiquitous but subtle. Prefer `transition-colors duration-200` to make state shifts feel polished rather than instantaneous.

---

## 2. Color Palette System

We exclusively utilize the Tailwind `zinc` scale for all neutral UI elements (backgrounds, borders, text).

| Purpose | Tailwind Classes | Notes |
|---------|------------------|-------|
| Main Background | `bg-zinc-50` | Use for layout, shells, and wide open spaces. |
| Foreground Cards | `bg-white` | For actionable boundaries, feed containers, settings panes. |
| Primary Text | `text-zinc-900` | Headings, essential copy, unselected navigation items. |
| Secondary Text | `text-zinc-500` | Descriptions, metadata, muted contexts. |
| Borders & Dividers | `border-zinc-200` or `ring-zinc-950/5` | Use sub-pixel rings where possible to simulate depth. |

### Accent Colors

Use accents sparingly and functionally. The primary brand color is a custom **electric cyan** ramp defined in `src/style.css` under `@theme` as `brand-*` tokens (500 is the vivid accent, 600 is tuned for text contrast).

- **Primary Accent:** `brand-600` (e.g., active states, progress bars, links)
- **Decorative Accent:** `brand-500` to `brand-400` (ambient blobs, logo marks — cyan family only, no multi-hue gradients)
- **Positive / Success:** `emerald-500` to `emerald-700` (success badges, Active status)
- **Danger / Destructive:** `rose-500` to `rose-700` (Danger Zone, delete account buttons)
- **Warning / Non-Destructive Pause:** `amber-500` to `amber-600` (Pause channel, pending states, rate limits)

Never reach for the framework default `indigo`/`violet`, and never use multi-hue decorative gradients (e.g. indigo→purple). A cyan-family gradient on a brand element is acceptable; a rainbow/other-hue gradient is not.

---

## 3. Typography Rules

Type is **IBM Plex Sans** (body) paired with **IBM Plex Mono** for numeric/data content — a deliberate technical pair, loaded via Google Fonts in `src/style.css`.

- **Body & Headings:** IBM Plex Sans via the `--font-sans` token (Tailwind `font-sans`). Use `font-semibold` or `font-bold` paired with `tracking-tight` (negative letter spacing) for headings.
- **Data / Numbers:** IBM Plex Mono via `font-mono` (plus `tabular-nums`). Apply to counts, metrics, percentages, prices, timestamps — anything numeric the user reads.
- **Metadata or System text:** Use `text-xs font-medium` for dates, user handles, and statuses.

---

## 4. Component Library Standards

When building new features, use these exact patterns for consistency:

### A. Layout Cards

```html
<div class="bg-white shadow-sm ring-1 ring-zinc-950/5 sm:rounded-2xl p-6 sm:p-8">
  <!-- Content here -->
</div>
```

### B. Standard Input Fields

Use slightly off-white backgrounds that reveal pure white upon focus to invite interaction.

```html
<input 
  class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm text-sm focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 transition-colors bg-zinc-50/50 focus:bg-white text-zinc-900" 
  placeholder="..." 
/>
```

### C. Standard Buttons

**Primary Button (High Emphasis):**
```html
<button class="inline-flex justify-center items-center py-2.5 px-5 border border-transparent rounded-xl shadow-sm text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 transition-colors duration-200 cursor-pointer">
  Action
</button>
```

**Secondary Button (Medium Emphasis):**
```html
<button class="inline-flex justify-center items-center py-2.5 px-5 border border-zinc-200 rounded-xl shadow-sm text-sm font-medium text-zinc-700 bg-white hover:bg-zinc-50 hover:border-zinc-300 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 transition-colors duration-200 cursor-pointer">
  Cancel
</button>
```

*(Note: Always ensure buttons and clickable interfaces trigger a pointer cursor natively or via custom styling globally.)*

### D. Status Badges

Badges should rely on high-contrast tinted relationships (e.g., `emerald-50` background with `emerald-700` text). A subtle ring ensures borders distinct regardless of background.

```html
<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset bg-emerald-50 text-emerald-700 ring-emerald-600/20">
  Active
</span>
```

---

## 5. Animations & Ambiance

Whenever there is open whitespace that feels empty, incorporate ambient, slowly pulsating or slow-moving gradient spheres in the brand cyan family.

```html
<!-- Ambient Decorator Element Example -->
<div class="absolute top-0 right-0 -mt-16 -mr-16 w-32 h-32 bg-gradient-to-br from-brand-500 to-brand-400 rounded-full blur-[60px] opacity-30 pointer-events-none"></div>
```

- Modals backgrounds should employ simple backdrops: `bg-zinc-900/60 backdrop-blur-sm transition-opacity`.
- Motion should be functional, not decorative: no hover-grow/scale on cards, no glowing shadows, and always respect `prefers-reduced-motion` (handled globally in `src/style.css`).

By adhering strictly to these components, values, and interaction guidelines, all engineers can produce designs indistinguishable from the core identity.
