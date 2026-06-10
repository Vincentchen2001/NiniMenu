import type { DayOverride, PlanProfile } from "@/types"
import { PROFILE_EMOJI, PROFILE_LABELS, PROTEIN_OPTIONS } from "@/lib/weekPlanCombos"
import { RefreshCw, X } from "lucide-react"

const PROFILE_KEYS: PlanProfile[] = ["balanced", "quick", "light", "spicy", "favorite", "soup"]

export default function DayThemeSheet({
  dayLabel,
  value,
  busy,
  canRegenerate,
  onChange,
  onRegenerateDay,
  onClose,
}: {
  dayKey: string
  dayLabel: string
  value: DayOverride
  busy: boolean
  canRegenerate: boolean
  onChange: (next: DayOverride) => void
  onRegenerateDay: () => void
  onClose: () => void
}) {
  const want = value.want || []

  function pickProfile(profile: PlanProfile | "") {
    onChange({ ...value, profile })
  }

  function toggleWant(key: string) {
    const next = want.includes(key) ? want.filter((item) => item !== key) : [...want, key]
    onChange({ ...value, want: next })
  }

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
              <div className="truncate text-lg font-extrabold">{dayLabel}吃什么口味</div>
              <div className="mt-0.5 text-[12px] font-medium text-text3">每周重复生效，改完点应用设置</div>
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
          <div className="mb-2 text-[13px] font-extrabold text-text">当天主题</div>
          <div className="mb-4 grid grid-cols-3 gap-2">
            <button
              onClick={() => pickProfile("")}
              className={`flex h-11 items-center justify-center gap-1.5 rounded-2xl border text-[13px] font-bold transition-all active:scale-95 ${
                !value.profile ? "border-primary bg-primary-light text-primary" : "border-border bg-bg text-text2"
              }`}
            >
              跟随默认
            </button>
            {PROFILE_KEYS.map((profile) => (
              <button
                key={profile}
                onClick={() => pickProfile(profile)}
                className={`flex h-11 items-center justify-center gap-1.5 rounded-2xl border text-[13px] font-bold transition-all active:scale-95 ${
                  value.profile === profile ? "border-primary bg-primary-light text-primary" : "border-border bg-bg text-text2"
                }`}
              >
                <span aria-hidden>{PROFILE_EMOJI[profile]}</span>
                {PROFILE_LABELS[profile]}
              </button>
            ))}
          </div>

          <div className="mb-1 text-[13px] font-extrabold text-text">这天想吃</div>
          <div className="mb-2 text-[11px] font-medium text-text3">选了的食材这天会优先安排</div>
          <div className="mb-5 flex flex-wrap gap-2">
            {PROTEIN_OPTIONS.map((option) => (
              <button
                key={option.key}
                onClick={() => toggleWant(option.key)}
                className={`rounded-full border px-3.5 py-1.5 text-[12px] font-bold transition-all active:scale-95 ${
                  want.includes(option.key) ? "border-primary bg-primary text-white" : "border-border bg-bg text-text2"
                }`}
              >
                {option.label}
              </button>
            ))}
          </div>

          <div className="grid grid-cols-2 gap-2">
            <button
              onClick={onRegenerateDay}
              disabled={busy || !canRegenerate}
              className="flex h-11 items-center justify-center gap-2 rounded-2xl bg-bg text-sm font-extrabold text-text2 transition-all hover:text-primary active:scale-95 disabled:opacity-45"
              title={canRegenerate ? "只重新生成这一天的菜" : "先生成一周菜单"}
            >
              <RefreshCw size={16} strokeWidth={2.4} className={busy ? "animate-spin" : ""} />
              仅这天重生成
            </button>
            <button
              onClick={onClose}
              className="flex h-11 items-center justify-center rounded-2xl bg-primary text-sm font-extrabold text-white transition-all active:scale-95"
            >
              完成
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
