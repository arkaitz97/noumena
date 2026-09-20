## ADDED Requirements

### Requirement: Design Token Contract
The system SHALL expose a single set of semantic design tokens as Tailwind v4 theme variables in `frontend/src/style.css`, covering surfaces, text, borders, accents, and focus rings, and SHALL resolve them from the existing `zinc-*` and `brand-*` ramps rather than introducing a new palette.

#### Scenario: Semantic tokens resolve to the Noumena palette
- **WHEN** a component uses a semantic utility such as `bg-card`, `text-muted-foreground`, `border-border`, or `ring-ring`
- **THEN** the rendered color SHALL match the corresponding Noumena neutral or brand value (e.g., card surface equals the white / `zinc-900` surface, border equals `zinc-200`, ring equals `brand-500`)

#### Scenario: Dark mode remaps tokens
- **WHEN** the `dark` class is present on the document root
- **THEN** every semantic token SHALL resolve to a dark-appropriate value with no dark-on-dark text and no mismatched surfaces

#### Scenario: No raw palette classes in ported primitives
- **WHEN** a ported component in `frontend/src/components/ui/` is reviewed
- **THEN** it SHALL reference semantic tokens for themeable surfaces, text, borders, and rings instead of hardcoded `zinc-*` / `brand-*` literals

### Requirement: Themed Component Primitives
The system SHALL provide a Vue component library under `frontend/src/components/ui/` that covers buttons, text inputs, selects, checkboxes, switches, radio groups, tabs, badges, tooltips, loaders, toasts, modals, popovers, context menus, and drawers, each built with `motion-v` for animation and `lucide-vue-next` for icons.

#### Scenario: Primitive variant selection
- **WHEN** a consumer renders a button, badge, or tabs primitive with a supported variant (e.g., primary/secondary/ghost, neutral/success/warning/danger, pill/segment/underline)
- **THEN** the component SHALL render the corresponding themed appearance without the consumer supplying color classes

#### Scenario: Shared class merge utility
- **WHEN** a consumer passes a custom `class` to a primitive
- **THEN** the component SHALL merge it with its internal classes using `cn()` (`clsx` + `tailwind-merge`) so that consumer overrides win without duplicated or conflicting utilities

#### Scenario: Existing call-sites migrated
- **WHEN** the change is complete
- **THEN** all in-repo consumers of the replaced primitives (native selects, checkboxes, dark-mode toggle, status badges, filter popover, workspace/user menus, segmented control, intent tabs, toast host, confirm modal, keyboard help modal, export dialog) SHALL use the design-system primitives, and the superseded components SHALL be removed

### Requirement: Motion Behavior
The system SHALL implement component animation through `motion-v` with a consistent spring/easing character, and SHALL NOT introduce animation on static, non-interactive content.

#### Scenario: Spring-driven interaction
- **WHEN** an interactive primitive that specifies motion (button press, switch thumb, tabs indicator, modal, toast, popover) changes state
- **THEN** the transition SHALL animate via a spring or the shared snappy easing, not a linear or instant change

#### Scenario: Reduced motion
- **WHEN** the user has `prefers-reduced-motion: reduce`
- **THEN** components SHALL collapse animation to a near-instant, non-translating state while remaining fully functional

### Requirement: Component Accessibility
The system SHALL preserve and standardize keyboard and screen-reader behavior across all ported primitives.

#### Scenario: Modal focus management
- **WHEN** a modal or drawer opens and then closes
- **THEN** focus SHALL move into the surface on open, be trapped while open, close on Escape, and be restored to the previously focused element on close

#### Scenario: Menu and tab keyboard operability
- **WHEN** a menu, context menu, or tab list has focus
- **THEN** items SHALL be reachable with arrow keys, activatable with Enter/Space, and expose correct ARIA roles (`menu`/`menuitem`, `tablist`/`tab` with `aria-selected`)

#### Scenario: Form control error announcement
- **WHEN** an input, select, checkbox, or radio is in an error state
- **THEN** it SHALL set `aria-invalid` and associate its message via `aria-describedby`, and the error text SHALL be announced

### Requirement: Component Source Ownership
The system SHALL own the ported component source within the repository rather than depending on a third-party runtime component package.

#### Scenario: Ported source is local
- **WHEN** the change is implemented
- **THEN** each design-system component SHALL exist as a tracked `.vue` file in `frontend/src/components/ui/`, and the only added runtime dependencies SHALL be `motion-v`, `clsx`, and `tailwind-merge`

#### Scenario: Upstream provenance recorded
- **WHEN** a ported component is added
- **THEN** its upstream beUI origin SHALL be recorded in `docs/beui-component-map.md`
