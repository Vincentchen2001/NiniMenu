import { useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { weekPlanApi } from "@/api"
import type { MenuRule } from "@/types"
import {
  CATEGORY_OPTIONS,
  SCOPE_OPTIONS,
  STRENGTH_OPTIONS,
  TYPE_OPTIONS,
  describeTemplate,
  type MenuRuleTemplate,
} from "@/lib/menuRuleTemplates"
import { X } from "lucide-react"
import toast from "react-hot-toast"

const INGREDIENT_KEY = "ingredient"

function SelectField({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: Array<{ key: string; label: string }>
  onChange: (key: string) => void
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-[11px] font-bold text-text3">{label}</span>
      <select
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="h-10 w-full rounded-[10px] border-[1.5px] border-border bg-bg px-2 text-sm outline-none transition-all focus:border-primary"
      >
        {options.map((option) => (
          <option key={option.key} value={option.key}>{option.label}</option>
        ))}
      </select>
    </label>
  )
}

export default function RuleSentenceBuilder({
  onCreate,
  onClose,
}: {
  onCreate: (rule: MenuRule) => void
  onClose: () => void
}) {
  const [type, setType] = useState<MenuRuleTemplate["type"]>("limit")
  const [scope, setScope] = useState<"meal" | "day" | "week">("meal")
  const [categoryKey, setCategoryKey] = useState<string>("egg")
  const [ingredientWord, setIngredientWord] = useState("")
  const [n, setN] = useState(1)
  const [strength, setStrength] = useState<"must" | "prefer">("prefer")
  const [checking, setChecking] = useState(false)

  const needsCategory = type !== "no_repeat"
  const needsScope = type === "limit" || type === "no_repeat"
  const needsN = type === "limit" || type === "no_repeat"
  const isIngredient = categoryKey === INGREDIENT_KEY

  const template = useMemo<MenuRuleTemplate>(() => {
    const category = isIngredient ? `ingredient:${ingredientWord.trim()}` : categoryKey
    return {
      type,
      ...(needsScope ? { scope } : {}),
      ...(needsCategory ? { category } : {}),
      ...(needsN ? { n } : {}),
      ...(needsScope ? { strength } : {}),
    }
  }, [type, scope, categoryKey, ingredientWord, n, strength, needsCategory, needsScope, needsN, isIngredient])

  const sentence = describeTemplate(template)

  async function handleCreate() {
    if (needsCategory && isIngredient && !ingredientWord.trim()) {
      toast.error("请输入食材关键词")
      return
    }
    const rule: MenuRule = {
      code: `custom_rule_${Date.now()}`,
      name: sentence.split(" · ")[0],
      description: "",
      enabled: true,
      scope: needsScope ? scope : "candidate",
      rule_kind: "score",
      severity: "soft",
      relaxable: true,
      expression: "",
      template: JSON.stringify(template),
      priority: 500,
      message: "",
    }
    setChecking(true)
    try {
      await weekPlanApi.validateRule(rule)
      onCreate(rule)
      toast.success("已加入草稿，保存规则后生效")
      onClose()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "规则无效，请调整后重试")
    } finally {
      setChecking(false)
    }
  }

  // Portaled to body so transformed layout ancestors cannot hijack the
  // fixed overlay's containing block.
  return createPortal(
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
              <div className="truncate text-lg font-extrabold">用一句话创建规则</div>
              <div className="mt-0.5 text-[12px] font-medium text-text3">选好条件，下面会实时预览这条规则</div>
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
          <div className="grid grid-cols-2 gap-2">
            <SelectField
              label="想做什么"
              value={type}
              options={TYPE_OPTIONS}
              onChange={(key) => setType(key as MenuRuleTemplate["type"])}
            />
            {needsScope && (
              <SelectField
                label="在什么范围"
                value={scope}
                options={SCOPE_OPTIONS}
                onChange={(key) => setScope(key as "meal" | "day" | "week")}
              />
            )}
            {needsCategory && (
              <SelectField
                label="哪类菜"
                value={categoryKey}
                options={[...CATEGORY_OPTIONS, { key: INGREDIENT_KEY, label: "含某种食材…" }]}
                onChange={setCategoryKey}
              />
            )}
            {needsN && (
              <label className="block">
                <span className="mb-1 block text-[11px] font-bold text-text3">{type === "limit" ? "最多几道" : "最多几次"}</span>
                <input
                  type="number"
                  min={1}
                  max={10}
                  value={n}
                  onChange={(event) => setN(Math.max(1, Math.min(10, Number(event.target.value) || 1)))}
                  className="h-10 w-full rounded-[10px] border-[1.5px] border-border bg-bg px-3 text-sm outline-none transition-all focus:border-primary"
                />
              </label>
            )}
            {needsScope && (
              <SelectField
                label="多严格"
                value={strength}
                options={STRENGTH_OPTIONS}
                onChange={(key) => setStrength(key as "must" | "prefer")}
              />
            )}
          </div>

          {needsCategory && isIngredient && (
            <label className="mt-2 block">
              <span className="mb-1 block text-[11px] font-bold text-text3">食材关键词</span>
              <input
                value={ingredientWord}
                onChange={(event) => setIngredientWord(event.target.value)}
                placeholder="如：香菜"
                className="h-10 w-full rounded-[10px] border-[1.5px] border-border bg-bg px-3 text-sm outline-none transition-all focus:border-primary"
              />
            </label>
          )}

          <div className="mt-4 rounded-2xl border border-primary/20 bg-primary-light/60 px-3 py-2.5">
            <div className="text-[11px] font-bold text-primary/70">规则预览</div>
            <div className="mt-0.5 text-[13px] font-extrabold leading-relaxed text-primary">{sentence || "请完善条件"}</div>
          </div>

          <button
            onClick={handleCreate}
            disabled={checking}
            className="mt-4 flex h-11 w-full items-center justify-center rounded-2xl bg-primary text-sm font-extrabold text-white transition-all active:scale-95 disabled:opacity-45"
          >
            {checking ? "校验中…" : "创建规则"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
