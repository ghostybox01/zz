/** Session-only target list bodies (for fleet deploy). Never persisted. */
const bodies = new Map<string, string>()

export function setListBody(listId: string, body: string): void {
  bodies.set(listId, body)
}

export function getListBody(listId: string): string | undefined {
  return bodies.get(listId)
}

export function deleteListBody(listId: string): void {
  bodies.delete(listId)
}

export function clearListBodies(): void {
  bodies.clear()
}
