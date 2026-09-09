// Roving-tabindex keyboard navigation for `role="tablist"` groups. Wire to
// the tablist element's @keydown. Arrow keys move focus between tabs,
// Home/End jump, and activating the focused tab fires its click handler.

export function onRovingTablistKeydown(e: KeyboardEvent): void {
  const tablist = e.currentTarget as HTMLElement
  const tabs = Array.from(tablist.querySelectorAll<HTMLElement>('[role="tab"]'))
  if (tabs.length === 0) return
  const idx = tabs.indexOf(document.activeElement as HTMLElement)
  let next = -1
  if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = idx + 1 < tabs.length ? idx + 1 : 0
  else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = idx > 0 ? idx - 1 : tabs.length - 1
  else if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = tabs.length - 1
  if (next < 0) return
  e.preventDefault()
  tabs[next].focus()
  tabs[next].click()
}
