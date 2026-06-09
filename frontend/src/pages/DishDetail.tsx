import { useState, useMemo } from "react"
import { createPortal } from "react-dom"
import { useParams, useNavigate } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { dishesApi, favoritesApi, recordsApi } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import { asArray } from "@/lib/utils"
import { getDishImageUrl, getDishEmoji } from "@/components/DishImage"
import PhotoViewer from "@/components/PhotoViewer"
import PageHeader, { HeaderIconButton } from "@/components/PageHeader"
import SectionHeader from "@/components/SectionHeader"
import DishCard from "@/components/DishCard"
import { cardShadow } from "@/components/Card"
import type { Dish, DishRecordsStats } from "@/types"
import toast from "react-hot-toast"
import { Pencil, Star } from "lucide-react"

function diffLabel(d: string) { return d === "easy" ? "简单" : d === "medium" ? "中等" : "困难" }
function mealLabel(m: string) {
  if (m === "lunch") return "🍳 午餐"
  if (m === "dinner") return "🍲 晚餐"
  return "🍳🍲 午餐+晚餐"
}

const moodEmojiMap: Record<string, string> = { yum: "😋", ok: "😐", no: "😵", great: "😋", meh: "😕" }
const homeMoodEmojiMap: Record<string, string> = { happy: "😊", tired: "😫", lazy: "😌", spicy: "🤤", healthy: "🌿" }
const homeMoodLabelMap: Record<string, string> = { happy: "开心", tired: "疲惫", lazy: "想偷懒", spicy: "想吃辣", healthy: "想养生" }

interface Ingredient { name: string; amount: string }
interface Step { text: string; time?: number; image?: string }

function normalizeIngredients(raw: unknown[]): Ingredient[] {
  return raw.map((item) => {
    if (typeof item === "string") return { name: item, amount: "" }
    if (typeof item === "object" && item !== null) return { name: (item as Record<string, unknown>).name as string || "", amount: (item as Record<string, unknown>).amount as string || "" }
    return { name: String(item), amount: "" }
  })
}

function normalizeSteps(raw: unknown[]): Step[] {
  return raw.map((item) => {
    if (typeof item === "string") return { text: item }
    if (typeof item === "object" && item !== null) return { text: (item as Record<string, unknown>).text as string || "", time: (item as Record<string, unknown>).time as number | undefined, image: (item as Record<string, unknown>).image as string | undefined }
    return { text: String(item) }
  })
}

function splitAmount(amount: string): { num: number; unit: string } | null {
  const m = amount.trim().match(/^(\d+(?:\.\d+)?)(.*)$/)
  if (!m) return null
  return { num: Number(m[1]), unit: m[2] }
}

function IngredientCard({ ing, scale }: { ing: Ingredient; scale: number }) {
  const parsed = splitAmount(ing.amount)
  const [num, setNum] = useState(parsed ? parsed.num : 0)
  const displayNum = parsed ? Math.round(num * scale * 100) / 100 : 0

  return (
    <div className="bg-card p-2.5 rounded-[10px] text-center text-[13px] border border-border transition-all">
      <div className="font-semibold mb-0.5">{ing.name}</div>
      {parsed ? (
        <div className="flex items-center justify-center gap-1.5 mt-0.5">
          <button onClick={() => setNum((n) => Math.max(0, Math.round((n - 1) * 100) / 100))}
            className="w-5 h-5 rounded-full bg-bg border border-border text-text2 flex items-center justify-center text-xs leading-none active:bg-primary-light">−</button>
          <span className="text-[11px] text-text2 min-w-[28px]">{scale === 1 ? num : displayNum}{parsed.unit}</span>
          <button onClick={() => setNum((n) => Math.round((n + 1) * 100) / 100)}
            className="w-5 h-5 rounded-full bg-bg border border-border text-text2 flex items-center justify-center text-xs leading-none active:bg-primary-light">+</button>
        </div>
      ) : (
        ing.amount && <div className="text-[11px] text-text2">{scale === 1 ? ing.amount : ing.amount}</div>
      )}
    </div>
  )
}

function formatRelativeDate(dateStr: string): string {
  const d = new Date(dateStr)
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const target = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const diff = Math.floor((today.getTime() - target.getTime()) / 86400000)
  if (diff === 0) return "今天"
  if (diff === 1) return "昨天"
  if (diff === 2) return "前天"
  if (diff < 7) return `${diff}天前`
  if (diff < 30) return `${Math.floor(diff / 7)}周前`
  const m = d.getMonth() + 1
  const day = d.getDate()
  if (d.getFullYear() === now.getFullYear()) return `${m}月${day}日`
  return `${d.getFullYear()}年${m}月${day}日`
}

function formatWeekday(dateStr: string): string {
  const weekdays = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]
  return weekdays[new Date(dateStr).getDay()]
}

function getMasteryLevel(count: number): { label: string; icon: string; color: string } {
  if (count >= 10) return { label: "大师", icon: "👨‍🍳", color: "text-yellow bg-yellow-light border-yellow/20" }
  if (count >= 6) return { label: "熟练", icon: "🔥", color: "text-primary bg-primary-light border-primary/20" }
  if (count >= 3) return { label: "入门", icon: "✨", color: "text-mint bg-mint-light border-mint/20" }
  return { label: "初学", icon: "🌱", color: "text-purple bg-purple-light border-purple/20" }
}

export default function DishDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const dishId = Number(id)
  const isLoggedIn = useAuthStore((s) => s.isLoggedIn)

  const [portionScale, setPortionScale] = useState(1)
  const [viewerPhotos, setViewerPhotos] = useState<string[]>([])
  const [viewerIdx, setViewerIdx] = useState(0)
  const [showViewer, setShowViewer] = useState(false)

  const { data: dish, isLoading } = useQuery({
    queryKey: ["dish", dishId],
    queryFn: () => dishesApi.get(dishId),
    enabled: !!dishId,
    staleTime: 0,
    gcTime: 0,
  })

  const { data: favDishes } = useQuery({ queryKey: ["favorites"], queryFn: () => favoritesApi.list(), staleTime: 0 })
  const isFav = favDishes?.some((d: Dish) => d.id === dishId) || false

  const { data: dishRecordsData } = useQuery({
    queryKey: ["dish-records", dishId],
    queryFn: () => dishesApi.records(dishId),
    enabled: !!dishId,
    staleTime: 0,
  })
  const dishRecords = useMemo(() => dishRecordsData?.records ?? [], [dishRecordsData?.records])
  const dishStats: DishRecordsStats = dishRecordsData?.stats || { total_count: 0, lunch_count: 0, dinner_count: 0, yum_percent: 0, ok_percent: 0, no_percent: 0, avg_rating: 0, last_date: "", avg_interval: 0 }

  const { data: dishesData } = useQuery({
    queryKey: ["dishes", "all"],
    queryFn: () => dishesApi.list({ pageSize: "100" }),
  })
  const allDishes = useMemo(() => dishesData?.items ?? [], [dishesData?.items])
  const sameCategoryDishes = useMemo(() => allDishes.filter((d: Dish) => d.category === dish?.category && d.id !== dishId).slice(0, 6), [allDishes, dish?.category, dishId])

  const favMut = useMutation({
    mutationFn: () => isFav ? favoritesApi.remove(dishId) : favoritesApi.add(dishId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["favorites"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success(isFav ? "已取消收藏" : "❤ 已收藏")
    },
  })

  const recordMut = useMutation({
    mutationFn: (mealType: string) => recordsApi.create({
      dish_id: dishId,
      dish_name: dish?.name || "",
      meal_type: mealType,
      meal_date: `${new Date().getFullYear()}-${String(new Date().getMonth()+1).padStart(2,"0")}-${String(new Date().getDate()).padStart(2,"0")}`,
    }),
    onSuccess: () => {
      toast.success("❤ 已记录！")
      qc.invalidateQueries({ queryKey: ["records"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      qc.invalidateQueries({ queryKey: ["dish-records", dishId] })
    },
    onError: (err: unknown) => {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      if (msg) toast.error(msg)
      else toast.error("记录失败")
    },
  })

  const allPhotos = useMemo(() => {
    const photos: { url: string; date: string }[] = []
    for (const r of dishRecords) {
      if (r.photo) {
        photos.push({ url: r.photo, date: r.meal_date })
      }
      for (const p of r.photos) {
        photos.push({ url: p, date: r.meal_date })
      }
    }
    return photos
  }, [dishRecords])

  if (isLoading) return <div className="p-8 text-center text-text2">加载中...</div>
  if (!dish) return <div className="p-8 text-center text-text2">菜品不存在</div>

  const ingredients: Ingredient[] = normalizeIngredients(asArray(dish.ingredients))
  const seasonings: Ingredient[] = normalizeIngredients(asArray(dish.seasonings))
  const steps: Step[] = normalizeSteps(asArray(dish.steps))
  const tasteTags = (dish.taste || "").split(",").map((t) => t.trim()).filter(Boolean)
  const dishTags = asArray<string>(dish.tags).filter(Boolean)
  const mastery = getMasteryLevel(dishStats.total_count)
  const hasMoodData = dishStats.yum_percent + dishStats.ok_percent + dishStats.no_percent > 0

  return (
    <div className="min-h-screen bg-bg">
      <PageHeader
        title="菜品详情"
        subtitle={dish.name}
        onBack={() => navigate(-1)}
        actions={
          isLoggedIn && (
            <HeaderIconButton onClick={() => navigate(`/admin/dishes/${dishId}`)} aria-label="编辑菜品">
              <Pencil size={17} strokeWidth={2.3} />
            </HeaderIconButton>
          )
        }
      />

      <div className="relative h-[260px] bg-gradient-to-br from-primary-light to-pink-light overflow-hidden">
        {getDishImageUrl(dish) ? (
          <img src={getDishImageUrl(dish)!} alt={dish.name} className="w-full h-full object-cover" />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-[80px]">{getDishEmoji(dish)}</div>
        )}
        <div className="absolute inset-x-0 bottom-0 h-20 bg-gradient-to-t from-bg to-transparent" />
        <button onClick={() => favMut.mutate()} className={`absolute bottom-4 right-5 w-11 h-11 rounded-full bg-card shadow-md flex items-center justify-center text-xl transition-all active:animate-heartbeat z-[1] ${isFav ? "text-primary" : ""}`}>
          {isFav ? "❤" : "♡"}
        </button>
      </div>

      <div className="px-5 py-5 max-w-[640px] mx-auto" style={{ paddingBottom: "calc(80px + env(safe-area-inset-bottom))" }}>
        <div className="text-2xl font-extrabold mb-2">{dish.name}</div>
        <div className="flex gap-2 flex-wrap mb-3">
          <span className="px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-primary-light text-primary">{dish.category}</span>
          <span className="px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-mint-light text-mint">{dish.cook_time}分钟</span>
          <span className="px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-primary-light text-primary">{diffLabel(dish.difficulty)}</span>
          {tasteTags.map((t) => (
            <span key={t} className="px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-pink-light text-pink">{t}</span>
          ))}
          <span className="px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-purple-light text-purple">{mealLabel(dish.meal_type)}</span>
        </div>

        {dishTags.length > 0 && (
          <div className="flex gap-1.5 flex-wrap mb-3">
            {dishTags.map((t) => (
              <span key={t} className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-bg text-text2 border border-border">#{t}</span>
            ))}
          </div>
        )}

        {dish.remark && (
          <div className={`bg-yellow-light rounded-xl p-3.5 mb-5 border border-yellow/20 ${cardShadow}`}>
            <div className="text-[13px] text-yellow font-semibold mb-1">💡 小贴士</div>
            <div className="text-[13px] text-text leading-relaxed">{dish.remark}</div>
          </div>
        )}

        {dishStats.total_count > 0 && (
          <div className={`bg-card rounded-2xl p-4 mb-5 border border-border ${cardShadow}`}>
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <span className="text-base font-bold">做菜记录</span>
                <span className={`px-2 py-0.5 rounded-full text-[10px] font-bold border ${mastery.color}`}>{mastery.icon} {mastery.label}</span>
              </div>
              <span className="text-[12px] text-text3">做了 {dishStats.total_count} 次</span>
            </div>

            <div className="grid grid-cols-3 gap-2 mb-3">
              <div className="text-center bg-bg rounded-lg py-2">
                <div className="text-base font-bold text-primary">{dishStats.lunch_count}</div>
                <div className="text-[10px] text-text3">午餐</div>
              </div>
              <div className="text-center bg-bg rounded-lg py-2">
                <div className="text-base font-bold text-mint">{dishStats.dinner_count}</div>
                <div className="text-[10px] text-text3">晚餐</div>
              </div>
              <div className="text-center bg-bg rounded-lg py-2">
                <div className="text-base font-bold text-text">{dishStats.avg_interval > 0 ? `${dishStats.avg_interval}天` : "-"}</div>
                <div className="text-[10px] text-text3">平均间隔</div>
              </div>
            </div>

            {dishStats.last_date && (
              <div className="text-[11px] text-text3 mb-3">上次做：{formatRelativeDate(dishStats.last_date)}</div>
            )}

            {hasMoodData && (
              <div className="mb-2">
                <div className="text-[11px] text-text3 mb-1.5">满意度</div>
                <div className="flex h-3 rounded-full overflow-hidden bg-bg">
                  {dishStats.yum_percent > 0 && <div className="bg-mint transition-all" style={{ width: `${dishStats.yum_percent}%` }} />}
                  {dishStats.ok_percent > 0 && <div className="bg-yellow transition-all" style={{ width: `${dishStats.ok_percent}%` }} />}
                  {dishStats.no_percent > 0 && <div className="bg-pink transition-all" style={{ width: `${dishStats.no_percent}%` }} />}
                </div>
                <div className="flex gap-3 mt-1.5">
                  {dishStats.yum_percent > 0 && <span className="text-[10px] text-text3 flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-mint" />😋 好吃 {dishStats.yum_percent}%</span>}
                  {dishStats.ok_percent > 0 && <span className="text-[10px] text-text3 flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-yellow" />😐 一般 {dishStats.ok_percent}%</span>}
                  {dishStats.no_percent > 0 && <span className="text-[10px] text-text3 flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-pink" />😵 不行 {dishStats.no_percent}%</span>}
                </div>
              </div>
            )}

            {dishStats.avg_rating > 0 && (
              <div className="flex items-center gap-1 mt-2">
                <span className="text-[11px] text-text3">平均评分</span>
                <div className="flex items-center gap-0.5">
                  {[1, 2, 3, 4, 5].map((s) => (
                    <Star key={s} size={12} className={s <= Math.round(dishStats.avg_rating) ? "text-yellow fill-yellow" : "text-text4"} />
                  ))}
                </div>
                <span className="text-[11px] font-semibold text-text">{dishStats.avg_rating.toFixed(1)}</span>
              </div>
            )}
          </div>
        )}

        {dish.video_url && (
          <button onClick={() => window.open(dish.video_url, "_blank", "noopener,noreferrer")}
            className="w-full mb-5 py-2.5 px-5 rounded-full text-sm font-semibold bg-pink-light text-pink border border-pink/30 transition-all active:scale-97 flex items-center justify-center gap-2">
            ▶ 观看教程视频
          </button>
        )}

        {(ingredients.length > 0 || seasonings.length > 0) && (
          <div className="mb-6">
            <div className="flex items-center justify-between mb-2.5">
              <div className="text-[15px] font-bold flex items-center gap-2"><span className="text-base">🥬</span> 食材</div>
              <div className="flex items-center gap-1 bg-bg rounded-full px-1 py-0.5 border border-border">
                {[1, 2, 3, 4].map((s) => (
                  <button key={s} onClick={() => setPortionScale(s)}
                    className={`px-2.5 py-0.5 rounded-full text-[11px] font-medium transition-all ${portionScale === s ? "bg-primary text-white" : "text-text3 active:bg-primary-light"}`}>
                    {s}人份
                  </button>
                ))}
              </div>
            </div>
            {ingredients.length > 0 && (
              <div className="mb-3">
                <div className="text-[12px] text-text3 font-semibold mb-1.5">配料</div>
                <div className="grid grid-cols-3 gap-2">
                  {ingredients.map((ing, i) => (
                    <IngredientCard key={`ing-${i}`} ing={ing} scale={portionScale} />
                  ))}
                </div>
              </div>
            )}
            {seasonings.length > 0 && (
              <div>
                <div className="text-[12px] text-text3 font-semibold mb-1.5">调料</div>
                <div className="grid grid-cols-3 gap-2">
                  {seasonings.map((s, i) => (
                    <IngredientCard key={`sea-${i}`} ing={s} scale={portionScale} />
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {steps.length > 0 && (
          <div className="mb-6">
            <div className="text-[15px] font-bold mb-2.5 flex items-center gap-2"><span className="text-base">📝</span> 制作步骤</div>
            <div>
              {steps.map((s, i) => (
                <div key={i} className="flex gap-3.5 py-3.5 border-b border-border last:border-0">
                  <div className="flex-shrink-0 w-8 h-8 rounded-full bg-primary text-white flex items-center justify-center text-[13px] font-bold">{i + 1}</div>
                  <div className="flex-1">
                    <div className="text-sm leading-relaxed pt-1">{s.text}</div>
                    {s.time && <div className="text-[11px] text-text3 mt-1">⏱ {s.time}分钟</div>}
                    {s.image && (
                      <div className="mt-2 rounded-lg overflow-hidden border border-border">
                        <img src={s.image!} alt="" className="w-full max-h-40 object-cover" loading="lazy" />
                      </div>
                    )}
                  </div>
                  <button
                    onClick={() => {
                      if ("speechSynthesis" in window) {
                        speechSynthesis.cancel()
                        const u = new SpeechSynthesisUtterance(s.text)
                        u.lang = "zh-CN"; u.rate = 0.9
                        speechSynthesis.speak(u)
                        toast("🔊 正在播报...")
                      }
                    }}
                    className="flex-shrink-0 w-8 h-8 rounded-full bg-mint-light text-mint flex items-center justify-center text-sm active:bg-mint active:text-white transition-all mt-1"
                  >▶</button>
                </div>
              ))}
            </div>
          </div>
        )}

        <button onClick={() => navigate(`/dishes/${dishId}/cook`)} className="w-full py-3.5 px-7 rounded-full text-base font-semibold bg-primary text-white transition-all active:scale-96 hover:bg-primary-dark mb-6">🍳 开始做菜</button>

        {dishRecords.length > 0 && (
          <div className="mb-6">
            <SectionHeader title={<span className="flex items-center gap-2">📜 做菜回忆</span>} />

            {allPhotos.length > 0 && (
              <div className="mb-4">
                <div className="text-[11px] text-text3 mb-2">📷 回忆照片 ({allPhotos.length})</div>
                <div className="flex gap-2 overflow-x-auto scrollbar-none pb-1">
                  {allPhotos.map((p, i) => (
                    <div
                      key={i}
                      onClick={() => { setViewerPhotos(allPhotos.map(x => x.url)); setViewerIdx(i); setShowViewer(true) }}
                      className="w-16 h-16 rounded-lg overflow-hidden flex-shrink-0 bg-card border border-border cursor-pointer active:scale-95 transition-transform shadow-sm"
                    >
                      <img src={p.url} alt="" className="w-full h-full object-cover" loading="lazy" />
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className="relative">
              <div className="absolute left-[15px] top-1 bottom-1 w-[2px] bg-border" />
              <div className="flex flex-col">
                {dishRecords.slice(0, 10).map((r) => (
                  <div key={r.id} className="relative pl-10 pb-4">
                    <div className="absolute left-[9px] top-1.5 w-[14px] h-[14px] rounded-full border-2 border-primary bg-card z-10 flex items-center justify-center">
                      <div className={`w-[6px] h-[6px] rounded-full ${r.meal_type === "lunch" ? "bg-primary" : "bg-mint"}`} />
                    </div>
                    <div className={`bg-card rounded-xl p-3 border border-border ${cardShadow}`}>
                      <div className="flex items-center justify-between mb-1.5">
                        <div className="flex items-center gap-2">
                          <span className="text-[13px] font-semibold">{formatRelativeDate(r.meal_date)}</span>
                          <span className="text-[10px] text-text3">{formatWeekday(r.meal_date)}</span>
                          <span className={`px-1.5 py-px rounded-full text-[10px] font-medium ${r.meal_type === "lunch" ? "bg-primary-light text-primary" : "bg-mint-light text-mint"}`}>
                            {r.meal_type === "lunch" ? "午餐" : "晚餐"}
                          </span>
                        </div>
                        {r.mood && (
                          <span className="text-lg">{moodEmojiMap[r.mood]}</span>
                        )}
                      </div>
                      <div className="flex items-center gap-2 flex-wrap">
                        {r.rating > 0 && (
                          <div className="flex items-center gap-0.5">
                            {[1, 2, 3, 4, 5].map((s) => (
                              <Star key={s} size={10} className={s <= r.rating ? "text-yellow fill-yellow" : "text-text4"} />
                            ))}
                          </div>
                        )}
                        {r.home_mood && (
                          <span className="text-[10px] text-text3 flex items-center gap-0.5 bg-bg px-1.5 py-px rounded-full">
                            {homeMoodEmojiMap[r.home_mood]} {homeMoodLabelMap[r.home_mood]}
                          </span>
                        )}
                        {r.remark && (
                          <span className="text-[11px] text-text2">"{r.remark}"</span>
                        )}
                      </div>
                      {(r.photo || r.photos.length > 0) && (
                        <div className="flex gap-1.5 mt-2 overflow-x-auto scrollbar-none">
                          {r.photo && (
                            <div
                              onClick={() => { const all = [r.photo, ...r.photos]; setViewerPhotos(all); setViewerIdx(0); setShowViewer(true) }}
                              className="w-14 h-14 rounded-md overflow-hidden flex-shrink-0 cursor-pointer active:scale-95 transition-transform border border-primary/20 ring-1 ring-primary/10"
                            >
                              <img src={r.photo} alt="" className="w-full h-full object-cover" loading="lazy" />
                            </div>
                          )}
                          {r.photos.slice(0, r.photo ? 3 : 4).map((p, pi) => (
                            <div
                              key={pi}
                              onClick={() => { const all = r.photo ? [r.photo, ...r.photos] : r.photos; setViewerPhotos(all); setViewerIdx(r.photo ? pi + 1 : pi); setShowViewer(true) }}
                              className="w-11 h-11 rounded-md overflow-hidden flex-shrink-0 cursor-pointer active:scale-95 transition-transform border border-border"
                            >
                              <img src={p} alt="" className="w-full h-full object-cover" loading="lazy" />
                            </div>
                          ))}
                          {(r.photo ? r.photos.length - 3 : r.photos.length - 4) > 0 && (
                            <div className="w-11 h-11 rounded-md flex-shrink-0 bg-bg border border-border2 flex items-center justify-center text-[10px] text-text3 font-medium">+{r.photo ? r.photos.length - 3 : r.photos.length - 4}</div>
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {sameCategoryDishes.length > 0 && (
          <div className="mb-6">
            <SectionHeader title={<span className="flex items-center gap-2">🍽 同类菜品</span>} />
            <div className="flex gap-3 overflow-x-auto scrollbar-none pb-1">
              {sameCategoryDishes.map((d: Dish) => (
                <div key={d.id} className="w-[130px] flex-shrink-0" onClick={() => navigate(`/dishes/${d.id}`)}>
                  <DishCard dish={d} />
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="fixed bottom-0 left-0 right-0 z-[210] bg-bg/92 backdrop-blur-xl px-5 py-3 flex gap-2.5 border-t border-border" style={{ paddingBottom: "calc(12px + env(safe-area-inset-bottom))" }}>
        <button onClick={() => recordMut.mutate("lunch")} className="flex-1 py-2.5 rounded-full text-sm font-semibold bg-primary text-white transition-all active:scale-96">🍳 中午吃这个</button>
        <button onClick={() => recordMut.mutate("dinner")} className="flex-1 py-2.5 rounded-full text-sm font-semibold bg-mint text-white transition-all active:scale-96">🍲 晚上吃这个</button>
      </div>

      {showViewer && viewerPhotos.length > 0 && createPortal(
        <PhotoViewer photos={viewerPhotos} idx={viewerIdx} setIdx={setViewerIdx} onClose={() => setShowViewer(false)} />,
        document.body
      )}
    </div>
  )
}
