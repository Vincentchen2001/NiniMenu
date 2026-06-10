import { useMemo, useState } from "react"
import type { DayOverride, PlanProfile } from "@/types"
import {
  DAY_KEYS,
  DAY_LABELS,
  PROFILE_EMOJI,
  WEEKDAY_PACKS,
  WEEKEND_PACKS,
  type ComboPack,
} from "@/lib/weekPlanCombos"
import { Check, X } from "lucide-react"

function PackRow({
  title,
  packs,
  selected,
  onSelect,
}: {
  title: string
  packs: ComboPack[]
  selected: string | null
  onSelect: (key: string | null) => void
}) {
  return (
    <div className="mb-4">
      <div className="mb-2 text-[13px] font-extrabold text-text">{title}</div>
      <div className="grid gap-2">
        {packs.map((pack) => {
          const active = selected === pack.key
          return (
            <button
              key={pack.key}
              onClick={() => onSelect(active ? null : pack.key)}
              className={`flex items-center gap-3 rounded-2xl border p-3 text-left transition-all active:scale-[.98] ${
                active ? "border-primary bg-primary-light" : "border-border bg-bg/60"
              }`}
            >
              <span className="min-w-0 flex-1">
                <span className={`block text-[14px] font-extrabold ${active ? "text-primary" : "text-text"}`}>{pack.label}</span>
                <span className="mt-0.5 block text-[11px] font-medium leading-relaxed text-text3">{pack.desc}</span>
              </span>
              <span
                className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full ${
                  active ? "bg-primary text-white" : "bg-bg text-text4"
                }`}
                aria-hidden
              >
                <Check size={13} strokeWidth={3} />
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

export default function ComboSheet({
  onApply,
  onClose,
}: {
  onApply: (weekdayKey: string | null, weekendKey: string | null) => void
  onClose: () => void
}) {
  const [weekdayKey, setWeekdayKey] = useState<string | null>(null)
  const [weekendKey, setWeekendKey] = useState<string | null>(null)

  const preview = useMemo(() => {
    const days: Record<string, DayOverride> = {}
    const weekdayPack = WEEKDAY_PACKS.find((pack) => pack.key === weekdayKey)
    const weekendPack = WEEKEND_PACKS.find((pack) => pack.key === weekendKey)
    Object.assign(days, weekdayPack?.days || {}, weekendPack?.days || {})
    return DAY_KEYS.map((dayKey) => ({
      dayKey,
      emoji: days[dayKey]?.profile ? PROFILE_EMOJI[days[dayKey].profile as PlanProfile] : "·",
    }))
  }, [weekdayKey, weekendKey])

  return (
    <div className="fixed inset-0 z-[220] flex items-end justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/42" />
      <div
        onClick={(event) => event.stopPropagation()}
        className="relative flex max-h-[86vh] w-full max-w-[640px] flex-col rounded-t-[28px] bg-card shadow-[0_-8px_30px_rgba(0,0,0,.16)] animate-fadeUp"
      >
        <div className="shrink-0 px-5 pt-3">
          <div className="mx-auto mb-4 h-1 w-11 rounded-full bg-border2" />
          <div className="mb-4 flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate text-lg font-extrabold">一键套用组合</div>
              <div className="mt-0.5 text-[12px] font-medium text-text3">工作日、周末各选一套，会覆盖对应几天的主题</div>
            </div>
            <button
              onClick={onClose}
              className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-bg text-text3 transition-all hover:text-primary active:scale-95"
              aria-label="关闭"
              title="关闭"
            >
              <X size={18} strokeWidth={2.35} />
            </button>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 pb-6">
          <PackRow title="工作日（周一到周五）" packs={WEEKDAY_PACKS} selected={weekdayKey} onSelect={setWeekdayKey} />
          <PackRow title="周末（周六周日）" packs={WEEKEND_PACKS} selected={weekendKey} onSelect={setWeekendKey} />

          <div className="mb-4 rounded-2xl border border-border bg-bg/60 p-3">
            <div className="mb-2 text-[12px] font-extrabold text-text2">套用后一周长这样</div>
            <div className="grid grid-cols-7 gap-1">
              {preview.map((cell) => (
                <div key={cell.dayKey} className="flex flex-col items-center gap-0.5 rounded-xl bg-card px-1 py-1.5">
                  <span className="text-[10px] font-extrabold text-text3">{DAY_LABELS[cell.dayKey]}</span>
                  <span className="text-[14px] leading-none" aria-hidden>{cell.emoji}</span>
                </div>
              ))}
            </div>
          </div>

          <button
            onClick={() => onApply(weekdayKey, weekendKey)}
            disabled={!weekdayKey && !weekendKey}
            className="flex h-11 w-full items-center justify-center gap-2 rounded-2xl bg-primary text-sm font-extrabold text-white transition-all active:scale-95 disabled:opacity-45"
          >
            套用组合
          </button>
        </div>
      </div>
    </div>
  )
}
