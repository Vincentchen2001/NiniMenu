import type { DayOverride, PlanProfile, WeekPlan as WeekPlanType } from "@/types"
import { DAY_KEYS, DAY_LABELS, PROFILE_EMOJI, PROFILE_LABELS } from "@/lib/weekPlanCombos"
import { Wand2 } from "lucide-react"

function localDateString(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`
}

export default function DayThemeStrip({
  days,
  overrides,
  defaults,
  onPick,
  onOpenCombos,
}: {
  days: WeekPlanType["days"]
  overrides: Record<string, DayOverride>
  defaults: { weekday: PlanProfile; weekend: PlanProfile }
  onPick: (dayKey: string) => void
  onOpenCombos: () => void
}) {
  const today = localDateString(new Date())

  return (
    <section className="mb-4 rounded-[24px] border border-border bg-card p-3 shadow-[0_1px_3px_rgba(0,0,0,.035),0_8px_24px_rgba(26,26,46,.05)]">
      <div className="mb-2 flex items-center justify-between gap-2 px-1">
        <div className="text-[13px] font-extrabold text-text">每天的口味主题</div>
        <button
          onClick={onOpenCombos}
          className="flex shrink-0 items-center gap-1.5 rounded-full bg-primary-light px-3 py-1.5 text-[12px] font-extrabold text-primary transition-all active:scale-95"
        >
          <Wand2 size={13} strokeWidth={2.6} />
          组合
        </button>
      </div>
      <div className="grid grid-cols-7 gap-1.5">
        {DAY_KEYS.map((dayKey, index) => {
          const override = overrides[dayKey]
          const fallback = index >= 5 ? defaults.weekend : defaults.weekday
          const isToday = days[index]?.date === today
          const hasTheme = Boolean(override?.profile)
          const hasWant = Boolean(override?.want?.length)
          return (
            <button
              key={dayKey}
              onClick={() => onPick(dayKey)}
              className={`relative flex flex-col items-center gap-0.5 rounded-2xl border px-1 py-2 transition-all active:scale-95 ${
                isToday
                  ? "border-primary bg-primary-light"
                  : hasTheme || hasWant
                    ? "border-primary/25 bg-card"
                    : "border-border bg-bg/60"
              }`}
              aria-label={`设置周${DAY_LABELS[dayKey]}的口味`}
              title={hasTheme && override?.profile ? PROFILE_LABELS[override.profile as PlanProfile] : `跟随默认（${PROFILE_LABELS[fallback]}）`}
            >
              <span className={`text-[11px] font-extrabold ${isToday ? "text-primary" : "text-text3"}`}>
                {DAY_LABELS[dayKey]}
              </span>
              <span className="text-[15px] leading-none" aria-hidden>
                {hasTheme && override?.profile ? PROFILE_EMOJI[override.profile as PlanProfile] ?? "·" : <span className="text-text4">·</span>}
              </span>
              {hasWant && <span className="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-mint" aria-hidden />}
            </button>
          )
        })}
      </div>
      <div className="mt-2 px-1 text-[11px] font-medium text-text3">点一天设置主题和想吃的，· 表示跟随默认口味</div>
    </section>
  )
}
