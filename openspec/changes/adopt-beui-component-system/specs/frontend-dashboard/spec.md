## ADDED Requirements

### Requirement: Design-system primitives for interactive controls
The system SHALL render interactive controls in the dashboard and settings views using the shared design-system primitives, and SHALL preserve the existing keyboard, screen-reader, and dark-mode-readability guarantees when doing so.

#### Scenario: Controls use design-system primitives
- **GIVEN** a user on any dashboard, settings, or auth view
- **WHEN** the view renders a button, text input, select, checkbox, switch, tab list, badge, tooltip, toast, modal, or menu
- **THEN** the control SHALL be provided by the `frontend/src/components/ui/` primitives rather than an ad-hoc or native element with bespoke styling

#### Scenario: Behavior preserved after migration
- **GIVEN** a user interacting with a migrated control
- **WHEN** the user operates it by keyboard or screen reader
- **THEN** focus order, Escape-to-close, focus trapping and restoration for overlays, and announced roles/labels SHALL remain equivalent to the pre-migration behavior

#### Scenario: Dark-mode readability retained
- **GIVEN** dark mode enabled
- **WHEN** a migrated control is displayed
- **THEN** it SHALL render with no dark-on-dark text and surfaces consistent with the dark token set
