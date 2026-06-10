import { useState } from "react"
import { Plus, X } from "lucide-react"
import toast from "react-hot-toast"

export default function BlockedIngredientsCard({
  value,
  canEdit,
  busy,
  onSave,
}: {
  value: string[]
  canEdit: boolean
  busy: boolean
  onSave: (next: string[]) => void
}) {
  const [input, setInput] = useState("")

  function addWords() {
    const words = input
      .split(/[\s，、,；;]+/)
      .map((word) => word.trim())
      .filter(Boolean)
    if (!words.length) {
      toast.error("请输入忌口食材")
      return
    }
    const next = [...value]
    words.forEach((word) => {
      if (!next.includes(word)) next.push(word)
    })
    setInput("")
    if (next.length !== value.length) onSave(next)
  }

  function removeWord(word: string) {
    onSave(value.filter((item) => item !== word))
  }

  return (
    <div className="rounded-2xl border border-border bg-bg/60 p-3">
      <div className="mb-1 text-[13px] font-extrabold text-text">🚫 家庭忌口</div>
      <div className="mb-2 text-[11px] font-medium text-text3">
        含这些食材的菜不会出现在任何推荐里（菜名、食材、调料都会检查）
      </div>
      {value.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {value.map((word) => (
            <span key={word} className="flex items-center gap-1 rounded-full bg-card px-2.5 py-1 text-[12px] font-bold text-text2 shadow-[inset_0_0_0_1px_rgba(26,26,46,.06)]">
              {word}
              {canEdit && (
                <button
                  onClick={() => removeWord(word)}
                  disabled={busy}
                  className="flex h-4 w-4 items-center justify-center rounded-full text-text3 transition-all hover:bg-red-light hover:text-red active:scale-90 disabled:opacity-45"
                  aria-label={`移除忌口${word}`}
                  title="移除"
                >
                  <X size={10} strokeWidth={2.7} />
                </button>
              )}
            </span>
          ))}
        </div>
      )}
      {canEdit && (
        <div className="flex gap-2">
          <input
            value={input}
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") addWords()
            }}
            placeholder="如：香菜 苦瓜（空格分隔可加多个）"
            className="h-10 min-w-0 flex-1 rounded-[10px] border-[1.5px] border-border bg-card px-3 text-sm outline-none transition-all focus:border-primary"
          />
          <button
            onClick={addWords}
            disabled={busy}
            className="flex h-10 shrink-0 items-center gap-1 rounded-full bg-primary px-3.5 text-xs font-semibold text-white transition-all active:scale-95 disabled:opacity-60"
          >
            <Plus size={13} strokeWidth={2.7} />
            添加
          </button>
        </div>
      )}
    </div>
  )
}
