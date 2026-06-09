import { useState } from "react"
import type { MenuRule } from "@/types"
import { CheckCircle2, Code2, Lock, Trash2 } from "lucide-react"

function menuRuleKindLabel(kind: string) {
  return kind === "score" ? "影响排序" : "限制搭配"
}

function menuRuleSeverityLabel(severity: string) {
  return severity === "soft" ? "可建议" : "必须遵守"
}

function menuRuleScopeLabel(scope: string) {
  switch (scope) {
    case "candidate":
      return "单道菜"
    case "day":
      return "同一天"
    case "week":
      return "整周"
    default:
      return "同一餐"
  }
}

function isNewDraftRule(rule: MenuRule) {
  return !rule.id && rule.code.startsWith("custom_rule_")
}

function menuRuleComparable(rule: MenuRule) {
  return {
    id: rule.id || 0,
    code: rule.code,
    name: rule.name,
    description: rule.description,
    enabled: rule.enabled,
    scope: rule.scope,
    rule_kind: rule.rule_kind,
    severity: rule.severity,
    relaxable: rule.relaxable,
    expression: rule.expression,
    priority: rule.priority,
    message: rule.message,
  }
}

export function menuRulesEqual(left: MenuRule[], right: MenuRule[]) {
  if (left.length !== right.length) return false
  return left.every((rule, index) => JSON.stringify(menuRuleComparable(rule)) === JSON.stringify(menuRuleComparable(right[index])))
}

export default function MenuRuleEditor({
  rules,
  canEdit,
  dirty,
  busy,
  onChange,
  onAdd,
  onRemove,
  onValidate,
  onSave,
}: {
  rules: MenuRule[]
  canEdit: boolean
  dirty: boolean
  busy: boolean
  onChange: (index: number, patch: Partial<MenuRule>) => void
  onAdd: () => void
  onRemove: (index: number) => void
  onValidate: (rule: MenuRule) => void
  onSave: () => void
}) {
  const [advancedOpen, setAdvancedOpen] = useState<Set<string>>(new Set())

  function toggleAdvanced(key: string) {
    setAdvancedOpen((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  return (
    <section className="rounded-2xl border border-border bg-bg/50 p-3">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[14px] bg-card text-text3">
            <Code2 size={18} strokeWidth={2.35} />
          </span>
          <div className="min-w-0">
            <div className="text-[15px] font-extrabold text-text">高级规则</div>
            <div className="truncate text-[11px] font-medium text-text3">{rules.length} 条 · {canEdit ? "管理员可调整" : "只读查看"}</div>
          </div>
        </div>
        <span className={`shrink-0 rounded-full px-2.5 py-1 text-[10px] font-extrabold ${canEdit ? "bg-mint-light text-mint" : "bg-card text-text3"}`}>
          {canEdit ? "ADMIN" : <span className="inline-flex items-center gap-1"><Lock size={11} />只读</span>}
        </span>
      </div>

      <div className="grid gap-2">
        {rules.map((rule, index) => {
          const key = rule.code || String(index)
          const isAdvancedOpen = advancedOpen.has(key)
          const removeLabel = isNewDraftRule(rule) ? "取消创建" : "删除规则"
          return (
            <div key={key} className="rounded-2xl border border-border bg-card p-3">
              <div className="mb-2 flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={rule.enabled}
                  disabled={!canEdit}
                  onChange={(event) => onChange(index, { enabled: event.target.checked })}
                  className="h-4 w-4 accent-primary"
                  aria-label="启用规则"
                />
                <input
                  value={rule.name}
                  disabled={!canEdit}
                  onChange={(event) => onChange(index, { name: event.target.value })}
                  className="min-w-0 flex-1 rounded-xl border border-border bg-bg px-3 py-2 text-sm font-extrabold text-text outline-none disabled:text-text2"
                />
                {canEdit && (
                  <button
                    onClick={() => onRemove(index)}
                    disabled={busy}
                    className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-bg text-text3 transition-all hover:bg-primary-light hover:text-primary active:scale-95 disabled:opacity-45"
                    aria-label={removeLabel}
                    title={removeLabel}
                  >
                    <Trash2 size={16} strokeWidth={2.35} />
                  </button>
                )}
                <button
                  onClick={() => onValidate(rule)}
                  disabled={!canEdit || busy}
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-bg text-text3 transition-all hover:text-primary disabled:opacity-45"
                  aria-label="校验规则"
                  title="校验规则"
                >
                  <CheckCircle2 size={17} strokeWidth={2.35} />
                </button>
              </div>

              <div className="mb-2 flex flex-wrap gap-1.5">
                <span className="rounded-full bg-bg px-2.5 py-1 text-[11px] font-bold text-text2">{menuRuleKindLabel(rule.rule_kind)}</span>
                <span className={`rounded-full px-2.5 py-1 text-[11px] font-bold ${rule.severity === "soft" ? "bg-bg text-text3" : "bg-primary-light text-primary"}`}>{menuRuleSeverityLabel(rule.severity)}</span>
                <span className="rounded-full bg-bg px-2.5 py-1 text-[11px] font-bold text-text2">{menuRuleScopeLabel(rule.scope)}</span>
                <span className={`rounded-full px-2.5 py-1 text-[11px] font-bold ${rule.relaxable ? "bg-mint-light text-mint" : "bg-bg text-text3"}`}>{rule.relaxable ? "可降级" : "不降级"}</span>
              </div>

              <input
                value={rule.description}
                disabled={!canEdit}
                onChange={(event) => onChange(index, { description: event.target.value })}
                className="mb-2 w-full rounded-xl border border-border bg-bg px-3 py-2 text-[12px] font-medium text-text2 outline-none disabled:text-text3"
                placeholder="这条规则用来解决什么搭配问题"
              />
              <input
                value={rule.message}
                disabled={!canEdit}
                onChange={(event) => onChange(index, { message: event.target.value })}
                className="w-full rounded-xl border border-border bg-bg px-3 py-2 text-[12px] font-bold text-text2 outline-none disabled:text-text3"
                placeholder="规则命中时给人的提示"
              />

              {canEdit && (
                <button
                  onClick={() => toggleAdvanced(key)}
                  className="mt-2 flex h-8 items-center justify-center rounded-xl bg-bg px-3 text-[12px] font-extrabold text-text3 transition-all hover:text-primary active:scale-95"
                >
                  {isAdvancedOpen ? "收起高级编辑" : "高级编辑"}
                </button>
              )}

              {canEdit && isAdvancedOpen && (
                <div className="mt-2 rounded-2xl border border-border bg-bg p-3">
                  <div className="mb-2 text-[11px] font-extrabold text-text3">技术表达式，只有需要改底层判断时才编辑</div>
                  <div className="mb-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
                    <input
                      value={rule.code}
                      onChange={(event) => onChange(index, { code: event.target.value })}
                      className="rounded-xl border border-border bg-card px-2.5 py-2 text-[12px] font-bold text-text2 outline-none"
                      placeholder="规则编码"
                    />
                    <select
                      value={rule.rule_kind}
                      onChange={(event) => onChange(index, { rule_kind: event.target.value })}
                      className="rounded-xl border border-border bg-card px-2.5 py-2 text-[12px] font-bold text-text2 outline-none"
                    >
                      <option value="constraint">约束</option>
                      <option value="score">打分</option>
                    </select>
                    <select
                      value={rule.severity}
                      onChange={(event) => onChange(index, { severity: event.target.value })}
                      className="rounded-xl border border-border bg-card px-2.5 py-2 text-[12px] font-bold text-text2 outline-none"
                    >
                      <option value="hard">硬规则</option>
                      <option value="soft">软规则</option>
                    </select>
                    <input
                      type="number"
                      value={rule.priority}
                      onChange={(event) => onChange(index, { priority: Number(event.target.value) })}
                      className="rounded-xl border border-border bg-card px-2.5 py-2 text-[12px] font-bold text-text2 outline-none"
                      aria-label="优先级"
                    />
                  </div>
                  <div className="mb-2 grid grid-cols-2 gap-2">
                    <select
                      value={rule.scope}
                      onChange={(event) => onChange(index, { scope: event.target.value })}
                      className="rounded-xl border border-border bg-card px-2.5 py-2 text-[12px] font-bold text-text2 outline-none"
                    >
                      <option value="candidate">候选</option>
                      <option value="meal">同餐</option>
                      <option value="day">同日</option>
                      <option value="week">全周</option>
                    </select>
                    <label className="flex min-h-9 items-center gap-2 rounded-xl border border-border bg-card px-2.5 py-2 text-[12px] font-bold text-text2">
                      <input
                        type="checkbox"
                        checked={rule.relaxable}
                        onChange={(event) => onChange(index, { relaxable: event.target.checked })}
                        className="h-4 w-4 accent-primary"
                      />
                      可降级
                    </label>
                  </div>
                  <textarea
                    value={rule.expression}
                    onChange={(event) => onChange(index, { expression: event.target.value })}
                    rows={2}
                    className="w-full rounded-xl border border-border bg-card px-3 py-2 font-mono text-[12px] leading-relaxed text-text outline-none"
                  />
                </div>
              )}
            </div>
          )
        })}
      </div>

      {canEdit && (
        <div className="mt-3 grid grid-cols-2 gap-2">
          <button onClick={onAdd} disabled={busy} className="flex h-10 items-center justify-center rounded-2xl bg-card text-sm font-extrabold text-text2 transition-all active:scale-95 disabled:opacity-45">
            新增规则
          </button>
          <button onClick={onSave} disabled={busy || !dirty} className="flex h-10 items-center justify-center rounded-2xl bg-primary text-sm font-extrabold text-white transition-all active:scale-95 disabled:opacity-45">
            保存规则
          </button>
        </div>
      )}
    </section>
  )
}
