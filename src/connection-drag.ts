import { useEffect, useRef, useState } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'

/** 触发拖拽需要的最小位移（px），避免与普通点击冲突 */
const DRAG_THRESHOLD = 4
/** 特殊放置目标：拖到「新建分组」入口，放下后弹出命名表单并把连接移入 */
export const NEW_GROUP_DROP = '__new-group__'

export type ConnectionDragController = {
  draggedId: string | null
  dropGroup: string | null
  dragProps: (id: string, name: string) => { onMouseDown: (event: ReactMouseEvent<HTMLElement>) => void }
}

/**
 * 自定义鼠标拖拽（不依赖 HTML5 原生 DnD）。
 * WKWebView 对 <button draggable> 的支持不可靠，这里用 mousedown/mousemove/mouseup 全程接管：
 * 按住移动超过阈值进入拖拽 → 移动中用 elementFromPoint 探测 data-drop-group 落点 → 松开落组。
 */
export function useConnectionDrag(moveConnection: (id: string, group: string) => void): ConnectionDragController {
  const [draggedId, setDraggedId] = useState<string | null>(null)
  const [dropGroup, setDropGroup] = useState<string | null>(null)
  const pending = useRef<{ id: string; name: string; x: number; y: number } | null>(null)
  const ghost = useRef<HTMLDivElement | null>(null)
  const move = useRef(moveConnection)
  move.current = moveConnection

  useEffect(() => {
    function dropGroupAt(x: number, y: number): string | null {
      const target = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-drop-group]')
      return target ? target.dataset.dropGroup ?? null : null
    }
    function clear() {
      pending.current = null
      ghost.current?.remove()
      ghost.current = null
      setDraggedId(null)
      setDropGroup(null)
    }
    function onMouseMove(event: MouseEvent) {
      const drag = pending.current
      if (!drag) return
      if (!ghost.current) {
        if (Math.abs(event.clientX - drag.x) < DRAG_THRESHOLD && Math.abs(event.clientY - drag.y) < DRAG_THRESHOLD) return
        const item = document.createElement('div')
        item.className = 'drag-ghost'
        item.textContent = drag.name
        document.body.appendChild(item)
        ghost.current = item
        setDraggedId(drag.id)
      }
      event.preventDefault()
      ghost.current!.style.transform = `translate(${event.clientX + 12}px, ${event.clientY + 12}px)`
      setDropGroup(dropGroupAt(event.clientX, event.clientY))
    }
    function onMouseUp(event: MouseEvent) {
      const drag = pending.current
      const started = !!ghost.current
      const group = drag && started ? dropGroupAt(event.clientX, event.clientY) : null
      clear()
      if (drag && started && group != null) move.current(drag.id, group)
    }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
    window.addEventListener('blur', clear)
    document.documentElement.addEventListener('mouseleave', clear)
    return () => {
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
      window.removeEventListener('blur', clear)
      document.documentElement.removeEventListener('mouseleave', clear)
    }
  }, [])

  function dragProps(id: string, name: string) {
    return {
      onMouseDown: (event: ReactMouseEvent<HTMLElement>) => {
        if (event.button !== 0) return
        pending.current = { id, name, x: event.clientX, y: event.clientY }
      },
    }
  }

  return { draggedId, dropGroup, dragProps }
}
