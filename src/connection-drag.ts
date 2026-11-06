import type { DragEvent } from 'react'

export function connectionDragProps(id: string, setDragged: (id: string | null) => void) {
  return {
    draggable: true,
    onDragStart: (event: DragEvent<HTMLElement>) => {
      event.dataTransfer.setData('application/x-shellink-connection', id)
      event.dataTransfer.effectAllowed = 'move'
      setDragged(id)
    },
    onDragEnd: () => setDragged(null),
  }
}
