import { useState, useEffect } from "react"
import { useParams, useNavigate } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { dishesApi, uploadApi, settingsApi, getUploadErrorMessage } from "@/api"
import { asArray } from "@/lib/utils"
import toast from "react-hot-toast"

const DEFAULT_CATEGORIES = ["家常菜", "川菜", "粤菜", "快手菜", "汤品", "主食", "小食"]
const DEFAULT_TASTES = ["酸", "甜", "辣", "鲜", "清淡", "咸鲜", "麻辣", "蒜香", "葱香", "酱香", "糖醋", "咖喱"]
const mealTypes = [
  { key: "lunch", label: "🍳 午餐" },
  { key: "dinner", label: "🍲 晚餐" },
  { key: "all", label: "通用" },
]
const difficulties = ["easy", "medium", "hard"]
const diffLabels = { easy: "简单", medium: "中等", hard: "困难" }
const dishRoleOptions = [
  { key: "meat", label: "荤菜" },
  { key: "veg", label: "素菜" },
  { key: "soup", label: "汤" },
  { key: "staple", label: "主食" },
  { key: "side", label: "配菜" },
]
const proteinOptions = [
  { key: "egg", label: "蛋类" },
  { key: "pork", label: "猪肉" },
  { key: "beef", label: "牛肉" },
  { key: "lamb", label: "羊肉" },
  { key: "poultry", label: "禽类" },
  { key: "seafood", label: "水产" },
  { key: "soy", label: "豆制品" },
]
const temperatureOptions = [
  { key: "hot", label: "热食" },
  { key: "cold", label: "冷/凉" },
  { key: "mixed", label: "冷热均可" },
]
const cookingMethodOptions = [
  { key: "stir_fry", label: "炒" },
  { key: "cold_mix", label: "凉拌" },
  { key: "simmer", label: "煮/炖" },
  { key: "steam", label: "蒸" },
  { key: "pan_fry", label: "煎" },
  { key: "deep_fry", label: "炸" },
  { key: "braise", label: "焖/卤" },
  { key: "roast", label: "烤" },
]
const levelLabels = ["低", "中", "高"]

interface Ingredient { name: string; amount: string }
interface Step { text: string; time?: number }

function parseList(raw: unknown, fallback: string[]): string[] {
  const arr = asArray<string>(raw).filter((x) => typeof x === "string")
  return arr.length > 0 ? arr : fallback
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mb-5">
      <div className="text-xs font-bold text-primary uppercase tracking-wider mb-2.5">{title}</div>
      <div className="bg-card rounded-2xl p-4 shadow-sm border border-border space-y-4">
        {children}
      </div>
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-[13px] font-semibold mb-1.5 text-text2">{label}</label>
      {hint && <div className="text-[11px] text-text3 mb-1">{hint}</div>}
      {children}
    </div>
  )
}

const inputCls = "w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary focus:shadow-[0_0_0_3px_rgba(232,115,74,.1)]"
const textareaCls = "w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none min-h-[80px] resize-y leading-relaxed transition-all focus:border-primary"

export default function AdminDishEdit() {
  const { id } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const dishId = id ? Number(id) : 0
  const isNew = !id || id === "new"

  const { data: dish } = useQuery({
    queryKey: ["dish", dishId],
    queryFn: () => dishesApi.get(dishId),
    enabled: !isNew && !!dishId,
  })

  const { data: settings } = useQuery({
    queryKey: ["settings"],
    queryFn: () => settingsApi.get(),
  })
  const categories = parseList(settings?.categories, DEFAULT_CATEGORIES)
  const tastes = parseList(settings?.tastes, DEFAULT_TASTES)

  const [name, setName] = useState("")
  const [category, setCategory] = useState("家常菜")
  const [mealType, setMealType] = useState("all")
  const [difficulty, setDifficulty] = useState("easy")
  const [tasteList, setTasteList] = useState<string[]>([])
  const [cookTime, setCookTime] = useState(15)
  const [ingredientsText, setIngredientsText] = useState("")
  const [seasoningsText, setSeasoningsText] = useState("")
  const [stepsText, setStepsText] = useState("")
  const [remark, setRemark] = useState("")
  const [imageUrl, setImageUrl] = useState("")
  const [images, setImages] = useState<string[]>([])
  const [videoUrl, setVideoUrl] = useState("")
  const [tags, setTags] = useState<string[]>([])
  const [tagInput, setTagInput] = useState("")
  const [sortOrder, setSortOrder] = useState(0)
  const [dishRole, setDishRole] = useState("side")
  const [proteinSources, setProteinSources] = useState<string[]>([])
  const [servingTemperature, setServingTemperature] = useState("hot")
  const [cookingMethods, setCookingMethods] = useState<string[]>([])
  const [spiceLevel, setSpiceLevel] = useState(0)
  const [richnessLevel, setRichnessLevel] = useState(0)
  const [carbLevel, setCarbLevel] = useState(0)
  const [addingCategory, setAddingCategory] = useState(false)
  const [addingTaste, setAddingTaste] = useState(false)
  const [newCategory, setNewCategory] = useState("")
  const [newTaste, setNewTaste] = useState("")
  const [manageCategory, setManageCategory] = useState(false)
  const [manageTaste, setManageTaste] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<{ type: "category" | "taste"; value: string } | null>(null)

  useEffect(() => {
    if (!dish) return
    setName(dish.name || "")
    setCategory(dish.category || "家常菜")
    setMealType(dish.meal_type || "all")
    setDifficulty(dish.difficulty || "easy")
    setTasteList((dish.taste || "").split(",").map((t) => t.trim()).filter(Boolean))
    setCookTime(dish.cook_time || 15)
    setRemark(dish.remark || "")
    setImageUrl(dish.image_url || "")
    setImages(asArray<string>(dish.images).filter((x) => typeof x === "string"))
    setVideoUrl(dish.video_url || "")
    setTags(asArray<string>(dish.tags).filter((x) => typeof x === "string"))
    setSortOrder(dish.sort_order || 0)
    setIngredientsText(asArray(dish.ingredients).map(fmtIngredient).join("\n"))
    setSeasoningsText(asArray(dish.seasonings).map(fmtIngredient).join("\n"))
    setStepsText(asArray(dish.steps).map(fmtStep).join("\n"))
    setDishRole(dish.dish_role || "side")
    setProteinSources(asArray<string>(dish.protein_sources).filter((x) => typeof x === "string"))
    setServingTemperature(dish.serving_temperature || "hot")
    setCookingMethods(asArray<string>(dish.cooking_methods).filter((x) => typeof x === "string"))
    setSpiceLevel(dish.spice_level || 0)
    setRichnessLevel(dish.richness_level || 0)
    setCarbLevel(dish.carb_level || 0)
  }, [dish])

  function fmtIngredient(item: unknown): string {
    if (typeof item === "string") return item
    if (typeof item === "object" && item !== null) {
      const o = item as Record<string, unknown>
      return `${o.name || ""} ${o.amount || ""}`.trim()
    }
    return String(item)
  }
  function fmtStep(item: unknown, i: number): string {
    if (typeof item === "string") return `${i + 1}. ${item}`
    if (typeof item === "object" && item !== null) {
      const o = item as Record<string, unknown>
      return `${i + 1}. ${o.text || ""}${o.time ? ` (${o.time}分钟)` : ""}`
    }
    return `${i + 1}. ${String(item)}`
  }

  function parseIngredients(text: string): Ingredient[] {
    return text.split("\n").filter((l) => l.trim()).map((l) => {
      const parts = l.trim().split(/\s+/)
      if (parts.length === 1) return { name: parts[0], amount: "" }
      return { name: parts.slice(0, -1).join(" "), amount: parts[parts.length - 1] }
    })
  }

  function parseSteps(text: string): Step[] {
    return text.split("\n").filter((l) => l.trim()).map((l) => {
      const cleaned = l.replace(/^\d+\.\s*/, "")
      const timeMatch = cleaned.match(/\((\d+)分钟\)/)
      return { text: cleaned.replace(/\(\d+分钟\)/, "").trim(), time: timeMatch ? Number(timeMatch[1]) : undefined }
    })
  }

  const saveMut = useMutation({
    mutationFn: () => {
      const data = {
        name,
        category,
        meal_type: mealType,
        difficulty,
        taste: tasteList.join(","),
        cook_time: cookTime,
        ingredients: JSON.stringify(parseIngredients(ingredientsText)),
        seasonings: JSON.stringify(parseIngredients(seasoningsText)),
        steps: JSON.stringify(parseSteps(stepsText)),
        remark,
        image_url: imageUrl,
        images: JSON.stringify(images),
        video_url: videoUrl,
        tags: JSON.stringify(tags),
        dish_role: dishRole,
        protein_sources: JSON.stringify(proteinSources),
        serving_temperature: servingTemperature,
        cooking_methods: JSON.stringify(cookingMethods),
        spice_level: spiceLevel,
        richness_level: richnessLevel,
        carb_level: carbLevel,
        trait_source: "manual",
        trait_version: 1,
        sort_order: sortOrder,
      }
      return isNew ? dishesApi.create(data) : dishesApi.update(dishId, data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "dishes"] })
      qc.invalidateQueries({ queryKey: ["dishes"] })
      toast.success("已保存")
      navigate(-1)
    },
    onError: () => toast.error("保存失败"),
  })

  const addCategoryMut = useMutation({
    mutationFn: (val: string) => settingsApi.update({ categories: JSON.stringify([...categories, val]) }),
    onSuccess: (_d, val) => {
      qc.invalidateQueries({ queryKey: ["settings"] })
      setCategory(val)
      setNewCategory("")
      setAddingCategory(false)
      toast.success("已添加分类")
    },
    onError: () => toast.error("添加失败，请先登录后台"),
  })

  const addTasteMut = useMutation({
    mutationFn: (val: string) => settingsApi.update({ tastes: JSON.stringify([...tastes, val]) }),
    onSuccess: (_d, val) => {
      qc.invalidateQueries({ queryKey: ["settings"] })
      setTasteList((prev) => prev.includes(val) ? prev : [...prev, val])
      setNewTaste("")
      setAddingTaste(false)
      toast.success("已添加口味")
    },
    onError: () => toast.error("添加失败，请先登录后台"),
  })

  const delCategoryMut = useMutation({
    mutationFn: (val: string) => settingsApi.update({ categories: JSON.stringify(categories.filter((c) => c !== val)) }),
    onSuccess: (_d, val) => {
      qc.invalidateQueries({ queryKey: ["settings"] })
      if (category === val) setCategory("")
      setPendingDelete(null)
      toast.success("已删除分类")
    },
    onError: () => toast.error("删除失败，请先登录后台"),
  })

  const delTasteMut = useMutation({
    mutationFn: (val: string) => settingsApi.update({ tastes: JSON.stringify(tastes.filter((t) => t !== val)) }),
    onSuccess: (_d, val) => {
      qc.invalidateQueries({ queryKey: ["settings"] })
      setTasteList((prev) => prev.filter((x) => x !== val))
      setPendingDelete(null)
      toast.success("已删除口味")
    },
    onError: () => toast.error("删除失败，请先登录后台"),
  })

  function confirmDelete() {
    if (!pendingDelete) return
    if (pendingDelete.type === "category") delCategoryMut.mutate(pendingDelete.value)
    else delTasteMut.mutate(pendingDelete.value)
  }

  function toggleTaste(t: string) {
    setTasteList((prev) => prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t])
  }

  function toggleListValue(value: string, setter: React.Dispatch<React.SetStateAction<string[]>>) {
    setter((prev) => prev.includes(value) ? prev.filter((x) => x !== value) : [...prev, value])
  }

  async function handleImageUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    try {
      const res = await uploadApi.image(file)
      setImageUrl(res.data.data.url)
      toast.success("主图已上传")
    } catch (err) {
      toast.error(getUploadErrorMessage(err))
    }
  }

  async function handleExtraImageUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const files = e.target.files
    if (!files) return
    for (const file of Array.from(files)) {
      try {
        const res = await uploadApi.image(file)
        setImages((prev) => [...prev, res.data.data.url])
      } catch (err) {
        toast.error(getUploadErrorMessage(err))
      }
    }
    toast.success("图片已上传")
  }

  function addTag() {
    const t = tagInput.trim()
    if (t && !tags.includes(t)) {
      setTags([...tags, t])
    }
    setTagInput("")
  }

  const pillClass = (active: boolean) =>
    `inline-flex items-center rounded-full text-xs border-[1.5px] transition-all ${active ? "bg-primary text-white border-primary" : "border-border text-text2"}`

  return (
    <div className="px-5 py-4 max-w-[640px] mx-auto pb-20">
      <div className="flex items-center justify-between mb-4">
        <div className="text-lg font-bold">{isNew ? "添加菜品" : "编辑菜品"}</div>
        <button onClick={() => saveMut.mutate()} disabled={!name || saveMut.isPending} className="py-1.5 px-5 rounded-full text-xs font-semibold bg-primary text-white transition-all active:scale-96 disabled:opacity-50">
          {saveMut.isPending ? "保存中..." : "保存菜品"}
        </button>
      </div>

      <Section title="基本信息">
        <Field label="菜品名称 *" hint="必填">
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="如：番茄炒蛋" className={inputCls} />
        </Field>

        <Field label="分类">
          <div className="flex items-center justify-between mb-1.5">
            <span />
            <button onClick={() => setManageCategory((v) => !v)} className={`text-[12px] px-2 py-0.5 rounded-full transition-all ${manageCategory ? "bg-primary text-white" : "text-text3 hover:text-primary"}`}>{manageCategory ? "完成" : "管理"}</button>
          </div>
          <div className="flex flex-wrap gap-2 items-center">
            {categories.map((c) => (
              <span key={c} className={pillClass(category === c && !manageCategory)}>
                <button onClick={() => !manageCategory && setCategory(c)} className="pl-3.5 pr-2 py-1.5 active:scale-95">{c}</button>
                {manageCategory && (
                  <button onClick={() => setPendingDelete({ type: "category", value: c })} className="pr-2.5 pl-0.5 py-1.5 text-text3 hover:text-red-500">✕</button>
                )}
              </span>
            ))}
            {addingCategory ? (
              <div className="flex items-center gap-1">
                <input autoFocus value={newCategory} onChange={(e) => setNewCategory(e.target.value)}
                  onKeyDown={(e) => { if (e.key === "Enter" && newCategory.trim()) addCategoryMut.mutate(newCategory.trim()); if (e.key === "Escape") setAddingCategory(false) }}
                  placeholder="新分类" className="w-24 px-2.5 py-1.5 rounded-full text-xs border-[1.5px] border-primary bg-bg outline-none" />
                <button onClick={() => newCategory.trim() && addCategoryMut.mutate(newCategory.trim())} className="px-2.5 py-1.5 rounded-full text-xs bg-primary text-white">✓</button>
                <button onClick={() => { setAddingCategory(false); setNewCategory("") }} className="px-2.5 py-1.5 rounded-full text-xs border border-border text-text2">✕</button>
              </div>
            ) : (
              <button onClick={() => setAddingCategory(true)} className="px-3.5 py-1.5 rounded-full text-xs border-[1.5px] border-dashed border-border2 text-text2">+ 自定义</button>
            )}
          </div>
        </Field>

        <div className="grid grid-cols-2 gap-4">
          <Field label="适合餐次">
            <div className="flex flex-wrap gap-2">
              {mealTypes.map((m) => (
                <button key={m.key} onClick={() => setMealType(m.key)} className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${mealType === m.key ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>{m.label}</button>
              ))}
            </div>
          </Field>
          <Field label="难度">
            <div className="flex flex-wrap gap-2">
              {difficulties.map((d) => (
                <button key={d} onClick={() => setDifficulty(d)} className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${difficulty === d ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>{diffLabels[d as keyof typeof diffLabels]}</button>
              ))}
            </div>
          </Field>
        </div>

        <Field label="口味（可多选）">
          <div className="flex items-center justify-between mb-1.5">
            <span />
            <button onClick={() => setManageTaste((v) => !v)} className={`text-[12px] px-2 py-0.5 rounded-full transition-all ${manageTaste ? "bg-primary text-white" : "text-text3 hover:text-primary"}`}>{manageTaste ? "完成" : "管理"}</button>
          </div>
          <div className="flex flex-wrap gap-2 items-center">
            {tastes.map((t) => (
              <span key={t} className={pillClass(tasteList.includes(t) && !manageTaste)}>
                <button onClick={() => !manageTaste && toggleTaste(t)} className="pl-3.5 pr-2 py-1.5 active:scale-95">{t}</button>
                {manageTaste && (
                  <button onClick={() => setPendingDelete({ type: "taste", value: t })} className="pr-2.5 pl-0.5 py-1.5 text-text3 hover:text-red-500">✕</button>
                )}
              </span>
            ))}
            {addingTaste ? (
              <div className="flex items-center gap-1">
                <input autoFocus value={newTaste} onChange={(e) => setNewTaste(e.target.value)}
                  onKeyDown={(e) => { if (e.key === "Enter" && newTaste.trim()) addTasteMut.mutate(newTaste.trim()); if (e.key === "Escape") setAddingTaste(false) }}
                  placeholder="新口味" className="w-24 px-2.5 py-1.5 rounded-full text-xs border-[1.5px] border-primary bg-bg outline-none" />
                <button onClick={() => newTaste.trim() && addTasteMut.mutate(newTaste.trim())} className="px-2.5 py-1.5 rounded-full text-xs bg-primary text-white">✓</button>
                <button onClick={() => { setAddingTaste(false); setNewTaste("") }} className="px-2.5 py-1.5 rounded-full text-xs border border-border text-text2">✕</button>
              </div>
            ) : (
              <button onClick={() => setAddingTaste(true)} className="px-3.5 py-1.5 rounded-full text-xs border-[1.5px] border-dashed border-border2 text-text2">+ 自定义</button>
            )}
          </div>
        </Field>

        <div className="grid grid-cols-2 gap-4">
          <Field label="烹饪时间（分钟）">
            <input type="number" value={cookTime} onChange={(e) => setCookTime(Number(e.target.value))} min={1} className={inputCls} />
          </Field>
          <Field label="排序权重" hint="数值越大越靠前">
            <input type="number" value={sortOrder} onChange={(e) => setSortOrder(Number(e.target.value))} className={inputCls} />
          </Field>
        </div>
      </Section>

      <Section title="图片与视频">
        <Field label="主图" hint="点击上传或替换菜品主图（支持 jpg/png/webp，最大5MB）">
          <label className="block border-2 border-dashed border-border2 rounded-2xl overflow-hidden text-center cursor-pointer transition-all hover:border-primary hover:bg-primary-light">
            <input type="file" accept="image/*" onChange={handleImageUpload} className="hidden" />
            {imageUrl ? (
              <img src={imageUrl} alt="预览" className="w-full h-48 object-cover" />
            ) : (
              <div className="p-6">
                <span className="text-[32px] block mb-2">📷</span>
                <div className="text-[13px] text-text2">点击上传主图</div>
              </div>
            )}
          </label>
        </Field>

        <Field label="额外图片" hint="可上传多张菜品图片">
          <label className="block border-2 border-dashed border-border2 rounded-xl overflow-hidden text-center cursor-pointer py-3 px-4 transition-all hover:border-primary hover:bg-primary-light">
            <input type="file" accept="image/*" multiple onChange={handleExtraImageUpload} className="hidden" />
            <span className="text-xs text-text2">+ 点击上传更多图片（可多选）</span>
          </label>
          {images.length > 0 && (
            <div className="flex gap-2 mt-3 overflow-x-auto scrollbar-hide">
              {images.map((img, i) => (
                <div key={i} className="relative flex-shrink-0 w-20 h-20 rounded-xl overflow-hidden group">
                  <img src={img} alt="" className="w-full h-full object-cover" />
                  <button
                    onClick={() => setImages(images.filter((_, j) => j !== i))}
                    className="absolute top-0.5 right-0.5 w-5 h-5 rounded-full bg-black/60 text-white text-xs flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity"
                  >✕</button>
                </div>
              ))}
            </div>
          )}
        </Field>

        <Field label="教程视频链接" hint="B站/抖音/YouTube 视频链接，点击可跳转">
          <input type="text" value={videoUrl} onChange={(e) => setVideoUrl(e.target.value)} placeholder="粘贴视频链接" className={inputCls} />
        </Field>
      </Section>

      <Section title="菜品画像">
        <Field label="菜单角色" hint="用于周菜单按荤、素、汤、主食分配槽位">
          <div className="flex flex-wrap gap-2">
            {dishRoleOptions.map((option) => (
              <button key={option.key} onClick={() => setDishRole(option.key)} className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${dishRole === option.key ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>{option.label}</button>
            ))}
          </div>
        </Field>

        <Field label="主蛋白来源" hint="用于避免同餐双蛋、同蛋白重复">
          <div className="flex flex-wrap gap-2">
            {proteinOptions.map((option) => (
              <button key={option.key} onClick={() => toggleListValue(option.key, setProteinSources)} className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${proteinSources.includes(option.key) ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>{option.label}</button>
            ))}
          </div>
        </Field>

        <div className="grid grid-cols-2 gap-4">
          <Field label="冷热形态">
            <div className="flex flex-wrap gap-2">
              {temperatureOptions.map((option) => (
                <button key={option.key} onClick={() => setServingTemperature(option.key)} className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${servingTemperature === option.key ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>{option.label}</button>
              ))}
            </div>
          </Field>
          <Field label="做法">
            <div className="flex flex-wrap gap-2">
              {cookingMethodOptions.map((option) => (
                <button key={option.key} onClick={() => toggleListValue(option.key, setCookingMethods)} className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${cookingMethods.includes(option.key) ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>{option.label}</button>
              ))}
            </div>
          </Field>
        </div>

        <div className="grid grid-cols-3 gap-3">
          {[
            { label: "辣度", value: spiceLevel, set: setSpiceLevel },
            { label: "厚重度", value: richnessLevel, set: setRichnessLevel },
            { label: "主食感", value: carbLevel, set: setCarbLevel },
          ].map((item) => (
            <Field key={item.label} label={item.label}>
              <div className="grid grid-cols-3 overflow-hidden rounded-full border border-border bg-bg">
                {[0, 1, 2].map((level) => (
                  <button
                    key={level}
                    onClick={() => item.set(level)}
                    className={`py-1.5 text-[11px] font-bold transition-all ${item.value === level ? "bg-primary text-white" : "text-text3 hover:text-primary"}`}
                  >
                    {levelLabels[level]}
                  </button>
                ))}
              </div>
            </Field>
          ))}
        </div>
      </Section>

      <Section title="食材与步骤">
        <Field label="配料清单" hint="每行：名称 数量，最后一段为数量">
          <textarea value={ingredientsText} onChange={(e) => setIngredientsText(e.target.value)} rows={4} placeholder={"番茄 2个\n鸡蛋 3个"} className={textareaCls} />
        </Field>

        <Field label="调料清单" hint="每行：名称 数量">
          <textarea value={seasoningsText} onChange={(e) => setSeasoningsText(e.target.value)} rows={2} placeholder={"盐 1茶匙\n白糖 1茶匙"} className={textareaCls} />
        </Field>

        <Field label="制作步骤" hint="每行一步，可选(分钟数)标注时间">
          <textarea value={stepsText} onChange={(e) => setStepsText(e.target.value)} rows={4} placeholder={"1. 番茄洗净切块 (3分钟)\n2. 倒入蛋液翻炒"} className={textareaCls} />
        </Field>
      </Section>

      <Section title="标签与备注">
        <Field label="标签" hint="添加自定义标签方便筛选">
          <div className="flex flex-wrap gap-2 mb-2">
            {tags.map((t) => (
              <span key={t} className="inline-flex items-center gap-1 px-3 py-1 rounded-full text-xs bg-primary-light text-primary font-semibold">
                {t}
                <button onClick={() => setTags(tags.filter((x) => x !== t))} className="hover:text-red-500">✕</button>
              </span>
            ))}
          </div>
          <div className="flex gap-2">
            <input
              type="text" value={tagInput} onChange={(e) => setTagInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); addTag() } }}
              placeholder="输入标签后回车添加" className={`flex-1 ${inputCls}`}
            />
            <button onClick={addTag} className="px-4 rounded-full text-xs font-semibold bg-primary text-white">添加</button>
          </div>
        </Field>

        <Field label="备注" hint="烹饪小贴士、注意事项等">
          <textarea value={remark} onChange={(e) => setRemark(e.target.value)} rows={2} placeholder="一些小贴士..." className={`${textareaCls} min-h-[60px]`} />
        </Field>
      </Section>

      <div className="flex gap-2.5 mt-2">
        <button onClick={() => navigate(-1)} className="flex-1 py-2.5 px-5 rounded-full text-sm font-semibold border-[1.5px] border-border2 transition-all">取消</button>
        <button onClick={() => saveMut.mutate()} disabled={!name || saveMut.isPending} className="flex-1 py-2.5 px-5 rounded-full text-sm font-semibold bg-primary text-white transition-all active:scale-96 disabled:opacity-50">
          {saveMut.isPending ? "保存中..." : "保存菜品"}
        </button>
      </div>

      {pendingDelete && (
        <div className="fixed inset-0 z-[300] flex items-center justify-center p-6 bg-black/40 backdrop-blur-sm" onClick={() => setPendingDelete(null)}>
          <div className="bg-card rounded-2xl p-5 w-full max-w-[320px] shadow-xl" onClick={(e) => e.stopPropagation()}>
            <div className="text-base font-bold mb-1.5">删除{pendingDelete.type === "category" ? "分类" : "口味"}</div>
            <div className="text-sm text-text2 mb-5">确定删除「{pendingDelete.value}」吗？此操作不可撤销。</div>
            <div className="flex gap-2.5">
              <button onClick={() => setPendingDelete(null)} className="flex-1 py-2.5 px-5 rounded-full text-sm font-semibold border-[1.5px] border-border2">取消</button>
              <button onClick={confirmDelete} disabled={delCategoryMut.isPending || delTasteMut.isPending} className="flex-1 py-2.5 px-5 rounded-full text-sm font-semibold bg-red-500 text-white disabled:opacity-50">删除</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
