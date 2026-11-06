const STORAGE_KEY = 'swcode.sidePanel';
const CLOSED = 'closed';

// Without storage, the choice lasts as long as the page.
const memory = { open: true };

/** Whether the session page shows its side panel (agents, llm-server, context): as people last left it. */
export function sidePanelOpen(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) !== CLOSED;
  } catch {
    return memory.open;
  }
}

/** Shows or hides the side panel, and keeps the choice for the next visit. */
export function setSidePanelOpen(open: boolean): void {
  memory.open = open;
  try {
    if (open) {
      localStorage.removeItem(STORAGE_KEY);
      return;
    }
    localStorage.setItem(STORAGE_KEY, CLOSED);
  } catch {
    // The choice stays in memory.
  }
}
