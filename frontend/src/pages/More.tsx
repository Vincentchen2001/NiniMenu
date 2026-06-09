import { useNavigate } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { achievementsApi, settingsApi, shoppingListApi } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import { asString } from "@/lib/utils"
import PageHeader from "@/components/PageHeader"
import type { Achievement, ShoppingCategory } from "@/types"
import toast from "react-hot-toast"
import { Gift, Lock, Menu, Settings, ShoppingBasket, Trophy, Volume2 } from "lucide-react"

function getTodayStr() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

export default function More() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const isLoggedIn = useAuthStore((s) => s.isLoggedIn)

  const { data: rawAchievements } = useQuery({ queryKey: ["achievements"], queryFn: () => achievementsApi.list() })
  const achievements = Array.isArray(rawAchievements) ? rawAchievements : []
  const { data: rawShoppingList } = useQuery({ queryKey: ["shopping-list"], queryFn: () => shoppingListApi.get(), staleTime: 0 })
  const shoppingList = rawShoppingList ?? []
  const { data: settings } = useQuery({ queryKey: ["settings"], queryFn: () => settingsApi.get() })

  const checkMut = useMutation({
    mutationFn: (data: { item_name: string; meal_date: string; checked: boolean }) =>
      shoppingListApi.toggle(data),
    onMutate: async (data) => {
      await qc.cancelQueries({ queryKey: ["shopping-list"] })
      const prev = qc.getQueryData<ShoppingCategory[]>(["shopping-list"])
      if (prev) {
        qc.setQueryData<ShoppingCategory[]>(["shopping-list"], prev.map((cat) => ({
          ...cat,
          items: cat.items.map((item) =>
            item.name === data.item_name ? { ...item, checked: data.checked } : item
          ),
        })))
      }
      return { prev }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["shopping-list"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
    },
    onError: (_err, _data, ctx) => {
      if (ctx?.prev) qc.setQueryData(["shopping-list"], ctx.prev)
      toast.error("买菜状态更新失败")
    },
  })

  const inventoryMut = useMutation({
    mutationFn: (data: { item_name: string; in_stock: boolean }) =>
      shoppingListApi.setInventory(data),
    onMutate: async (data) => {
      await qc.cancelQueries({ queryKey: ["shopping-list"] })
      const prev = qc.getQueryData<ShoppingCategory[]>(["shopping-list"])
      if (prev) {
        qc.setQueryData<ShoppingCategory[]>(["shopping-list"], prev.map((cat) => ({
          ...cat,
          items: cat.items.map((item) =>
            item.name === data.item_name ? { ...item, in_stock: data.in_stock } : item
          ),
        })))
      }
      return { prev }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["shopping-list"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
    },
    onError: (_err, _data, ctx) => {
      if (ctx?.prev) qc.setQueryData(["shopping-list"], ctx.prev)
      toast.error("库存状态更新失败")
    },
  })

  const settingsMut = useMutation({
    mutationFn: (s: Record<string, string>) => settingsApi.update(s),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings"] })
      toast.success("已更新")
    },
  })

  function toggleSetting(key: string, currentVal: string) {
    settingsMut.mutate({ [key]: currentVal === "1" ? "0" : "1" })
  }

  const todayStr = getTodayStr()
  const unlockedCount = achievements.filter((a: Achievement) => a.is_unlocked).length
  const previewAchievements = achievements
    .slice()
    .sort((a: Achievement, b: Achievement) => {
      if (a.is_unlocked !== b.is_unlocked) return a.is_unlocked ? -1 : 1
      return a.id - b.id
    })
    .slice(0, 3)

  const totalChecked = shoppingList.reduce((acc: number, cat: ShoppingCategory) => acc + cat.items.filter((item) => item.checked).length, 0)
  const totalInStock = shoppingList.reduce((acc: number, cat: ShoppingCategory) => acc + cat.items.filter((item) => item.in_stock).length, 0)
  const totalItems = shoppingList.reduce((acc: number, cat: ShoppingCategory) => acc + cat.items.length, 0)
  const needBuyItems = shoppingList.reduce((acc: number, cat: ShoppingCategory) => acc + cat.items.filter((item) => !item.checked && !item.in_stock).length, 0)

  return (
    <div className="animate-fadeUp">
      <PageHeader title="更多" subtitle="买菜清单和设置" icon={Menu} />

      <div className="mx-auto max-w-[640px] px-5 py-4">
        <section className="mb-7">
          <div className="mb-3.5 flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[14px] bg-primary-light text-primary">
                <Trophy size={19} strokeWidth={2.35} />
              </span>
              <div className="min-w-0">
                <div className="text-lg font-extrabold text-text">成就墙</div>
                <div className="truncate text-[12px] font-medium text-text3">{unlockedCount}/{achievements.length} 已解锁</div>
              </div>
            </div>
          </div>
          <div className="mb-2 grid grid-cols-3 gap-3">
            {previewAchievements.map((a: Achievement) => (
              <button
                key={a.id}
                onClick={() => navigate("/achievements")}
                className={`relative overflow-hidden rounded-2xl border bg-card p-4 pt-5 text-center shadow-[0_1px_3px_rgba(0,0,0,.04),0_4px_12px_rgba(0,0,0,.04)] transition-all active:scale-95 ${a.is_unlocked ? "border-primary/20" : "border-border opacity-35 grayscale-[.9]"}`}
              >
                {a.is_unlocked && <div className="absolute left-0 right-0 top-0 h-[3px] bg-primary" />}
                <span className="mb-1.5 block text-[32px]">{a.icon}</span>
                <div className="mb-0.5 truncate text-xs font-semibold">{a.name}</div>
                <div className="line-clamp-2 text-[10px] text-text2">{a.description}</div>
              </button>
            ))}
          </div>
          <button
            onClick={() => navigate("/achievements")}
            className="w-full rounded-xl bg-primary-light/50 py-2 text-sm font-medium text-primary transition-all active:scale-98"
          >
            查看更多成就 ›
          </button>
        </section>

        <section className="mb-7">
          <div className="mb-3.5 flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[14px] bg-mint-light text-mint">
                <ShoppingBasket size={19} strokeWidth={2.35} />
              </span>
              <div className="min-w-0">
                <div className="text-lg font-extrabold text-text">买菜清单</div>
                <div className="truncate text-[12px] font-medium text-text3">
                  {totalItems > 0 ? `待买 ${needBuyItems} · 已买 ${totalChecked} · 家中 ${totalInStock}` : "暂无待买食材"}
                </div>
              </div>
            </div>
          </div>
          <div className="overflow-hidden rounded-2xl border border-border bg-card shadow-[0_1px_3px_rgba(0,0,0,.04),0_4px_12px_rgba(0,0,0,.04)]">
            {shoppingList.length === 0 ? (
              <div className="px-4 py-5 text-center text-sm text-text3">暂无买菜清单，点菜后自动生成今日和明日食材</div>
            ) : (
              shoppingList.map((cat: ShoppingCategory, ci: number) => (
                <div key={cat.category || ci} className={`px-4 py-3 ${ci > 0 ? "border-t border-border" : ""}`}>
                  <div className="mb-2 text-[13px] font-bold tracking-wide text-text2">{cat.category}</div>
                  {cat.items.map((item, ii) => (
                    <div key={`${item.name}-${ii}`} className={`flex items-center gap-2.5 py-1.5 text-sm ${item.in_stock ? "opacity-70" : ""}`}>
                      <button
                        onClick={() => checkMut.mutate({ item_name: item.name, meal_date: todayStr, checked: !item.checked })}
                        disabled={item.in_stock}
                        className={`flex h-[22px] w-[22px] flex-shrink-0 items-center justify-center rounded-md border-2 text-xs transition-all ${item.checked ? "border-mint bg-mint text-white" : item.in_stock ? "border-yellow bg-yellow-light text-yellow" : "border-border2 text-transparent"}`}
                        aria-label={item.checked ? "取消已买" : "标记已买"}
                      >
                        {item.in_stock ? "家" : "✓"}
                      </button>
                      <span className={`min-w-0 flex-1 truncate transition-all ${item.checked || item.in_stock ? "text-text3 line-through" : ""}`}>{item.name}</span>
                      <div className="ml-auto flex flex-shrink-0 items-center gap-1.5">
                        <span className="text-xs text-text3">{item.amount}</span>
                        <button
                          onClick={() => inventoryMut.mutate({ item_name: item.name, in_stock: !item.in_stock })}
                          disabled={inventoryMut.isPending}
                          className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold transition-all active:scale-95 disabled:opacity-60 ${item.in_stock ? "border-yellow/30 bg-yellow-light text-yellow" : "border-border bg-bg text-text3"}`}
                        >
                          {item.in_stock ? "家中有" : "库存"}
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              ))
            )}
          </div>
        </section>

        {isLoggedIn && (
          <section className="mb-7">
            <div className="mb-3.5 flex items-center gap-2">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[14px] bg-bg text-text3">
                <Settings size={19} strokeWidth={2.35} />
              </span>
              <div className="text-lg font-extrabold text-text">设置</div>
            </div>
            <div className="overflow-hidden rounded-2xl border border-border bg-card shadow-[0_1px_3px_rgba(0,0,0,.04),0_4px_12px_rgba(0,0,0,.04)]">
              <button onClick={() => toast(`推荐去重天数：${asString(settings?.repeat_days, "3")}天`)} className="flex w-full items-center justify-between border-b border-border px-4 py-3.5 text-left transition-all active:bg-bg">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-[10px] bg-primary-light text-primary">
                    <RefreshDaysIcon />
                  </div>
                  <div>
                    <div className="text-sm font-medium">推荐去重</div>
                    <div className="text-[11px] text-text2">近{asString(settings?.repeat_days, "3")}天吃过和推荐过的菜不优先出现</div>
                  </div>
                </div>
                <span className="flex items-center gap-1 text-sm text-text3">{asString(settings?.repeat_days, "3")}天 ›</span>
              </button>
              <button onClick={() => toggleSetting("voice_enabled", asString(settings?.voice_enabled, "1"))} className="flex w-full items-center justify-between border-b border-border px-4 py-3.5 text-left transition-all active:bg-bg">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-[10px] bg-mint-light text-mint">
                    <Volume2 size={17} strokeWidth={2.35} />
                  </div>
                  <div>
                    <div className="text-sm font-medium">语音播报</div>
                    <div className="text-[11px] text-text2">做菜时语音朗读步骤</div>
                  </div>
                </div>
                <div className={`relative h-6 w-11 rounded-xl transition-all ${settings?.voice_enabled !== "0" ? "bg-primary" : "bg-border2"}`}>
                  <div className={`absolute top-[2px] h-5 w-5 rounded-full bg-white shadow-sm transition-all ${settings?.voice_enabled !== "0" ? "left-[22px]" : "left-[2px]"}`} />
                </div>
              </button>
              <button onClick={() => toggleSetting("blind_box_enabled", asString(settings?.blind_box_enabled, "1"))} className="flex w-full items-center justify-between border-b border-border px-4 py-3.5 text-left transition-all active:bg-bg">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-[10px] bg-purple-light text-purple">
                    <Gift size={17} strokeWidth={2.35} />
                  </div>
                  <div>
                    <div className="text-sm font-medium">惊喜盲盒</div>
                    <div className="text-[11px] text-text2">首页显示盲盒推荐</div>
                  </div>
                </div>
                <div className={`relative h-6 w-11 rounded-xl transition-all ${settings?.blind_box_enabled !== "0" ? "bg-primary" : "bg-border2"}`}>
                  <div className={`absolute top-[2px] h-5 w-5 rounded-full bg-white shadow-sm transition-all ${settings?.blind_box_enabled !== "0" ? "left-[22px]" : "left-[2px]"}`} />
                </div>
              </button>
              <button onClick={() => navigate(isLoggedIn ? "/admin/dashboard" : "/admin/login")} className="flex w-full items-center justify-between px-4 py-3.5 text-left transition-all active:bg-bg">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-[10px] bg-pink-light text-primary">
                    <Lock size={17} strokeWidth={2.35} />
                  </div>
                  <div>
                    <div className="text-sm font-medium">管理模式</div>
                    <div className="text-[11px] text-text2">管理菜品、推荐语、成就</div>
                  </div>
                </div>
                <span className="text-sm text-text3">›</span>
              </button>
            </div>
          </section>
        )}
      </div>
    </div>
  )
}

function RefreshDaysIcon() {
  return <span className="text-sm font-extrabold leading-none">7</span>
}
