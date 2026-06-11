import type { MenuRule } from "@/types"
import { describeTemplate, withTemplate, type MenuRuleTemplate } from "@/lib/menuRuleTemplates"
import { Trash2 } from "lucide-react"

function NStepper({
  value,
  disabled,
  onChange,
  min = 1,
  max = 10,
}: {
  value: number
  disabled: boolean
  onChange: (next: number) => void
  min?: number
  max?: number
}) {
  return (
    <div className="grid h-7 w-[72px] shrink-0 grid-cols-3 overflow-hidden rounded-full border border-border bg-bg">
      <button
        onClick={() => onChange(value - 1)}
        disabled={disabled || value <= min}
        className="flex items-center justify-center text-base font-extrabold leading-none text-text3 transition-all hover:text-primary disabled:opacity-30"
        aria-label="减少"
      >
        -
      </button>
      <div className="flex items-center justify-center border-x border-border/70 text-[12px] font-extrabold text-text">{value}</div>
      <button
        onClick={() => onChange(value + 1)}
        disabled={disabled || value >= max}
        className="flex items-center justify-center text-base font-extrabold leading-none text-text3 transition-all hover:text-primary disabled:opacity-30"
        aria-label="增加"
      >
        +
      </button>
    </div>
  )
}

export default function PresetRuleCard({
  rule,
  tpl,
  canEdit,
  onChange,
  onRemove,
}: {
  rule: MenuRule
  tpl: MenuRuleTemplate
  canEdit: boolean
  onChange: (patch: Partial<MenuRule>) => void
  onRemove?: () => void
}) {
  const hasStepper = tpl.type === "limit" || tpl.type === "no_repeat"
  const hasPoints = tpl.type === "prefer" || tpl.type === "avoid" || (tpl.type === "limit" && tpl.strength !== "must")
  const pointsValue = tpl.points && tpl.points > 0 ? tpl.points : tpl.type === "prefer" ? 20 : 15 // 与后端 defaultTemplatePoints 保持一致（prefer 20 / limit·avoid 15）

  function updateN(next: number) {
    const clamped = Math.max(1, Math.min(10, next))
    onChange({ template: withTemplate(rule, { ...tpl, n: clamped }).template })
  }

  function updatePoints(next: number) {
    const clamped = Math.max(1, Math.min(99, next))
    onChange({ template: withTemplate(rule, { ...tpl, points: clamped }).template })
  }

  return (
    <div className={`rounded-2xl border p-3 transition-all ${rule.enabled ? "border-border bg-bg/60" : "border-border/60 bg-bg/30 opacity-70"}`}>
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <div className="text-[13px] font-extrabold text-text">{rule.name}</div>
          <div className="mt-1 text-[12px] font-medium leading-relaxed text-text2">{describeTemplate(tpl)}</div>
          {rule.description && <div className="mt-1 text-[11px] font-medium leading-relaxed text-text3">{rule.description}</div>}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {onRemove && canEdit && (
            <button
              onClick={onRemove}
              className="flex h-7 w-7 items-center justify-center rounded-full bg-bg text-text3 transition-all hover:bg-red-light hover:text-red active:scale-90"
              aria-label={`删除规则${rule.name}`}
              title="删除"
            >
              <Trash2 size={13} strokeWidth={2.4} />
            </button>
          )}
          <button
            onClick={() => onChange({ enabled: !rule.enabled })}
            disabled={!canEdit}
            className={`relative h-6 w-11 shrink-0 cursor-pointer rounded-xl transition-all ${rule.enabled ? "bg-primary" : "bg-border2"}`}
            aria-label={rule.enabled ? "停用规则" : "启用规则"}
          >
            <span className={`absolute top-[2px] h-5 w-5 rounded-full bg-white shadow-sm transition-all ${rule.enabled ? "left-[22px]" : "left-[2px]"}`} />
          </button>
        </div>
      </div>
      {hasStepper && (
        <div className="mt-2 flex items-center justify-between gap-2 rounded-xl bg-card px-3 py-2">
          <span className="text-[11px] font-bold text-text3">{tpl.type === "limit" ? "最多几道" : "最多几次"}</span>
          <NStepper value={tpl.n && tpl.n > 0 ? tpl.n : 1} disabled={!canEdit || !rule.enabled} onChange={updateN} />
        </div>
      )}
      {hasPoints && (
        <div className="mt-2 flex items-center justify-between gap-2 rounded-xl bg-card px-3 py-2">
          <span className="text-[11px] font-bold text-text3">{tpl.type === "avoid" ? "降多少分" : "加多少分"}</span>
          <NStepper value={pointsValue} disabled={!canEdit || !rule.enabled} onChange={updatePoints} min={1} max={99} />
        </div>
      )}
    </div>
  )
}
