import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { settingsApi, shoppingCategoriesApi } from "@/api"
import { asString } from "@/lib/utils"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import type { ShoppingCategoryOverride } from "@/types"
import toast from "react-hot-toast"

const shoppingCategories = ["蔬菜", "肉类", "配料", "其他"]

function Toggle({ value, onChange }: { value: boolean; onChange: () => void }) {
  return (
    <button
      onClick={onChange}
      className={`w-11 h-6 rounded-xl relative cursor-pointer transition-all flex-shrink-0 ${value ? "bg-primary" : "bg-border2"}`}
    >
      <div className={`absolute top-[2px] w-5 h-5 rounded-full bg-white shadow-sm transition-all ${value ? "left-[22px]" : "left-[2px]"}`} />
    </button>
  )
}

function SettingRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between py-3 border-b border-border/60 last:border-b-0">
      <div className="text-sm font-medium text-text">{label}</div>
      <div className="flex items-center gap-3">{children}</div>
    </div>
  )
}

export default function AdminSettings() {
  const qc = useQueryClient()
  const { data: settings, isLoading } = useQuery({ queryKey: ["settings"], queryFn: () => settingsApi.get() })

  const [repeatDays, setRepeatDays] = useState<string | null>(null)
  const [lunchPerDay, setLunchPerDay] = useState<string | null>(null)
  const [dinnerPerDay, setDinnerPerDay] = useState<string | null>(null)
  const [appName, setAppName] = useState<string | null>(null)
  const [shoppingItemName, setShoppingItemName] = useState("")
  const [shoppingCategory, setShoppingCategory] = useState("蔬菜")
  const storedAppName = useAppInfoStore((s) => s.appName)
  const updateAppName = useAppInfoStore((s) => s.setAppName)

  const updateMut = useMutation({
    mutationFn: (s: Record<string, string>) => settingsApi.update(s),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ["settings"] })
      if (variables.lunch_dishes_per_day !== undefined || variables.dinner_dishes_per_day !== undefined) {
        qc.invalidateQueries({ queryKey: ["week-plan"] })
        qc.invalidateQueries({ queryKey: ["shopping-list"] })
      }
      if (variables.app_name !== undefined) {
        updateAppName(variables.app_name || "NiniMenu")
      }
      toast.success("已保存")
    },
  })

  const { data: categoryOverrides = [] } = useQuery({
    queryKey: ["shopping-categories"],
    queryFn: () => shoppingCategoriesApi.list(),
  })

  const saveCategoryMut = useMutation({
    mutationFn: (data: { item_name: string; category: string }) => shoppingCategoriesApi.save(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["shopping-categories"] })
      qc.invalidateQueries({ queryKey: ["shopping-list"] })
      setShoppingItemName("")
      toast.success("分类已保存")
    },
    onError: () => toast.error("分类保存失败"),
  })

  const deleteCategoryMut = useMutation({
    mutationFn: (itemName: string) => shoppingCategoriesApi.delete(itemName),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["shopping-categories"] })
      qc.invalidateQueries({ queryKey: ["shopping-list"] })
      toast.success("已恢复智能识别")
    },
    onError: () => toast.error("删除失败"),
  })

  function saveShoppingCategory() {
    const itemName = shoppingItemName.trim()
    if (!itemName) {
      toast.error("请输入食材名称")
      return
    }
    saveCategoryMut.mutate({ item_name: itemName, category: shoppingCategory })
  }

  if (isLoading) return <div className="p-8 text-center text-text2">加载中...</div>

  return (
    <div className="px-5 py-4 max-w-[640px] mx-auto pb-20 space-y-4">
      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">基本设置</div>
        <div className="text-[11px] text-text3 mb-2">应用名称与显示偏好</div>

        <SettingRow label="应用名称">
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={appName !== null ? appName : asString(settings?.app_name, storedAppName)}
              onChange={(e) => setAppName(e.target.value)}
              placeholder="NiniMenu"
              className="w-28 py-2 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary text-right"
            />
            <button
              onClick={() => updateMut.mutate({ app_name: (appName !== null ? appName : asString(settings?.app_name, "NiniMenu")) || "NiniMenu" })}
              disabled={updateMut.isPending}
              className="px-3 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
            >
              保存
            </button>
          </div>
        </SettingRow>

        <SettingRow label="推荐去重天数">
          <div className="flex items-center gap-2">
            <input
              type="text"
              inputMode="numeric"
              value={repeatDays !== null ? repeatDays : asString(settings?.repeat_days, "")}
              onChange={(e) => setRepeatDays(e.target.value.replace(/[^0-9]/g, ""))}
              placeholder="3"
              className="w-16 py-2 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary text-right"
            />
            <span className="text-xs text-text3 whitespace-nowrap">天内不重复</span>
            <button
              onClick={() => {
                const v = repeatDays !== null ? repeatDays : asString(settings?.repeat_days, "")
                if (!v || Number(v) < 1) { toast.error("至少需要1天"); return }
                updateMut.mutate({ repeat_days: v })
              }}
              disabled={updateMut.isPending}
              className="px-3 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
            >
              保存
            </button>
          </div>
        </SettingRow>

        <SettingRow label="语音播报">
          <Toggle
            value={settings?.voice_enabled !== "0"}
            onChange={() => updateMut.mutate({ voice_enabled: settings?.voice_enabled === "1" ? "0" : "1" })}
          />
        </SettingRow>

        <SettingRow label="盲盒功能">
          <Toggle
            value={settings?.blind_box_enabled !== "0"}
            onChange={() => updateMut.mutate({ blind_box_enabled: settings?.blind_box_enabled === "1" ? "0" : "1" })}
          />
        </SettingRow>
      </div>

      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">一周菜单</div>
        <div className="text-[11px] text-text3 mb-2">设定每日午/晚餐推荐菜品数量</div>

        <SettingRow label="每日午餐数量">
          <div className="flex items-center gap-2">
            <input
              type="text"
              inputMode="numeric"
              value={lunchPerDay !== null ? lunchPerDay : asString(settings?.lunch_dishes_per_day, "")}
              onChange={(e) => setLunchPerDay(e.target.value.replace(/[^0-9]/g, ""))}
              placeholder="1"
              className="w-16 py-2 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary text-right"
            />
            <span className="text-xs text-text3">道菜</span>
            <button
              onClick={() => {
                const v = lunchPerDay !== null ? lunchPerDay : asString(settings?.lunch_dishes_per_day, "")
                if (v === "" || Number(v) < 0) { toast.error("不能小于0道菜"); return }
                updateMut.mutate({ lunch_dishes_per_day: v })
              }}
              disabled={updateMut.isPending}
              className="px-3 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
            >
              保存
            </button>
          </div>
        </SettingRow>

        <SettingRow label="每日晚餐数量">
          <div className="flex items-center gap-2">
            <input
              type="text"
              inputMode="numeric"
              value={dinnerPerDay !== null ? dinnerPerDay : asString(settings?.dinner_dishes_per_day, "")}
              onChange={(e) => setDinnerPerDay(e.target.value.replace(/[^0-9]/g, ""))}
              placeholder="1"
              className="w-16 py-2 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary text-right"
            />
            <span className="text-xs text-text3">道菜</span>
            <button
              onClick={() => {
                const v = dinnerPerDay !== null ? dinnerPerDay : asString(settings?.dinner_dishes_per_day, "")
                if (v === "" || Number(v) < 0) { toast.error("不能小于0道菜"); return }
                updateMut.mutate({ dinner_dishes_per_day: v })
              }}
              disabled={updateMut.isPending}
              className="px-3 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
            >
              保存
            </button>
          </div>
        </SettingRow>
      </div>

      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">买菜分类修正</div>
        <div className="text-[11px] text-text3 mb-3">识别不准时，在这里固定食材分类；删除后恢复智能识别。</div>
        <div className="flex gap-2 mb-3">
          <input
            value={shoppingItemName}
            onChange={(e) => setShoppingItemName(e.target.value)}
            placeholder="食材名称，如番茄"
            className="flex-1 min-w-0 py-2.5 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
          />
          <select
            value={shoppingCategory}
            onChange={(e) => setShoppingCategory(e.target.value)}
            className="w-20 py-2.5 px-2 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
          >
            {shoppingCategories.map((category) => (
              <option key={category} value={category}>{category}</option>
            ))}
          </select>
          <button
            onClick={saveShoppingCategory}
            disabled={saveCategoryMut.isPending}
            className="px-4 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
          >
            保存
          </button>
        </div>
        {categoryOverrides.length === 0 ? (
          <div className="text-xs text-text3 py-2">暂无手动修正</div>
        ) : (
          <div className="flex flex-col gap-1.5">
            {categoryOverrides.map((item: ShoppingCategoryOverride) => (
              <div key={item.item_name} className="flex items-center gap-2 py-1.5 border-t border-border first:border-t-0">
                <span className="text-sm font-medium text-text">{item.item_name}</span>
                <span className="text-[11px] text-primary bg-primary-light px-2 py-0.5 rounded-full">{item.category}</span>
                <button
                  onClick={() => {
                    setShoppingItemName(item.item_name)
                    setShoppingCategory(item.category)
                  }}
                  className="ml-auto text-[11px] text-text2 px-2 py-1 rounded-full bg-bg active:scale-95"
                >
                  修改
                </button>
                <button
                  onClick={() => deleteCategoryMut.mutate(item.item_name)}
                  disabled={deleteCategoryMut.isPending}
                  className="text-[11px] text-red px-2 py-1 rounded-full bg-bg active:scale-95 disabled:opacity-60"
                >
                  删除
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">密码来源</div>
        <div className="text-xs text-text3 leading-relaxed">
          应用密码由环境变量 APP_PASSWORD 控制，管理密码由 ADMIN_PASSWORD 控制。修改后重启服务生效。
        </div>
      </div>
    </div>
  )
}
