export interface DishIngredient {
  name: string
  amount?: string
}

export interface DishStep {
  text: string
  time?: number
  image?: string
}

export interface Dish {
  id: number
  name: string
  image_url: string
  // 后端返回真正的 JSON 数组；历史数据可能是字符串数组或对象数组，统一用 asArray 消费
  images: string[]
  video_url: string
  category: string
  meal_type: string
  taste: string
  ingredients: Array<string | DishIngredient>
  seasonings: Array<string | DishIngredient>
  steps: Array<string | DishStep>
  dish_role: string
  protein_sources: string[]
  serving_temperature: string
  cooking_methods: string[]
  spice_level: number
  richness_level: number
  carb_level: number
  trait_source: string
  trait_version: number
  cook_time: number
  difficulty: string
  remark: string
  favorite: boolean
  enabled: boolean
  tags: string[]
  sort_order: number
  created_at: string
  updated_at: string
}

// 创建/更新菜品的请求体：JSON 数组字段以字符串形式提交，与后端 CreateDishRequest 对应
export interface DishInput {
  name: string
  image_url?: string
  images?: string
  video_url?: string
  category?: string
  meal_type?: string
  taste?: string
  ingredients?: string
  seasonings?: string
  steps?: string
  dish_role?: string
  protein_sources?: string
  serving_temperature?: string
  cooking_methods?: string
  spice_level?: number
  richness_level?: number
  carb_level?: number
  trait_source?: string
  trait_version?: number
  cook_time?: number
  difficulty?: string
  remark?: string
  tags?: string
  sort_order?: number
}

export interface DishTraitAuditIssue {
  field: string
  label: string
  severity: string
  current: string
  expected: string
  message: string
}

export interface DishTraitAuditItem {
  dish_id: number
  dish_name: string
  category: string
  trait_source: string
  trait_version: number
  issues: DishTraitAuditIssue[]
}

export interface DishTraitAuditFieldCount {
  field: string
  label: string
  count: number
}

export interface DishTraitAuditReport {
  total: number
  passed: number
  issue_dishes: number
  issue_count: number
  auto_traits: number
  manual_traits: number
  field_counts: DishTraitAuditFieldCount[]
  items: DishTraitAuditItem[]
}

export interface MealRecord {
  id: number
  dish_id: number
  dish_name: string
  dish_image_url: string
  dish_emoji: string
  meal_type: string
  meal_date: string
  rating: number
  remark: string
  mood: string
  photo: string
  created_at: string
}

export interface PickResult {
  dishes: Dish[]
  quote: string
}

export interface BlindBoxResult {
  hint: string
  dish: Dish
  quote: string
}

export interface PaginatedData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

export interface StatsData {
  total_records: number
  total_dishes: number
  lunch_count: number
  dinner_count: number
  top_dishes: { dish_name: string; count: number }[]
}

export interface DashboardData {
  total_dishes: number
  enabled_dishes: number
  disabled_dishes: number
  total_records: number
  today_records: number
  favorite_count: number
  category_counts: { category: string; count: number }[]
  top_dishes: { dish_id: number; dish_name: string; count: number }[]
  recent_records: { id: number; dish_id: number; dish_name: string; meal_type: string; meal_date: string; mood: string; rating: number }[]
  week_trend: { date: string; count: number }[]
  difficulty_counts: { difficulty: string; count: number }[]
}

export interface Achievement {
  id: number
  code: string
  name: string
  description: string
  icon: string
  condition: string
  is_unlocked: boolean
  unlocked_at: string | null
  created_at: string
}

export interface WeekDayPlan {
  date: string
  day_name: string
  lunch: Dish[]
  dinner: Dish[]
}

export interface WeekPlan {
  days: WeekDayPlan[]
  warnings?: string[]
}

export type PlanProfile = "balanced" | "quick" | "light" | "spicy" | "favorite" | "soup"

export interface MealQuota {
  meat_count: number
  veg_count: number
  soup_count: number
}

export interface WeekPlanPeriodPreferences {
  profile: PlanProfile
  lunch: MealQuota
  dinner: MealQuota
}

export interface DayOverride {
  profile?: PlanProfile | ""
  want?: string[]
}

export interface WeekPlanPreferences {
  weekday: WeekPlanPeriodPreferences
  weekend: WeekPlanPeriodPreferences
  week_want?: string[]
  days?: Record<string, DayOverride>
}

export interface MenuRule {
  id?: number
  code: string
  name: string
  description: string
  enabled: boolean
  scope: string
  rule_kind: string
  severity: string
  relaxable: boolean
  expression: string
  template?: string
  priority: number
  message: string
  created_at?: string
  updated_at?: string
}

export interface ShoppingCategory {
  category: string
  items: ShoppingItem[]
}

export interface ShoppingItem {
  name: string
  amount: string
  checked: boolean
  in_stock: boolean
}

export interface ShoppingCategoryOverride {
  item_name: string
  category: string
}

export interface Holiday {
  id: number
  name: string
  date: string
  dish_ids: string
  greeting: string
  created_at: string
}

export interface Quote {
  id: number
  content: string
  scene: string
  enabled: boolean
  created_at: string
}

export interface DayRating {
  id: number
  meal_date: string
  home_mood: string
  mood: string
  remark: string
  photos: string
  created_at: string
  updated_at: string
}

export interface PhotoWallRecord {
  dish_id: number
  dish_name: string
  meal_type: string
  mood: string
  remark: string
}

export interface PhotoWallDay {
  meal_date: string
  photos: string[]
  home_mood: string
  day_mood: string
  day_remark: string
  records: PhotoWallRecord[]
  photo_count: number
}

export interface PhotoWall {
  days: PhotoWallDay[]
  total_days: number
  total_photos: number
}

export interface DishRecordWithDay {
  id: number
  meal_type: string
  meal_date: string
  rating: number
  remark: string
  mood: string
  photo: string
  home_mood: string
  day_mood: string
  day_remark: string
  photos: string[]
}

export interface CategoryCount {
  category: string
  count: number
}

export interface DishCategoryCounts {
  total: number
  categories: CategoryCount[]
}

export interface FavoriteOverviewItem {
  dish: Dish
  favorite_created_at: string
  record_count: number
  lunch_count: number
  dinner_count: number
  last_eaten_at: string
  avg_rating: number
  is_never_cooked: boolean
}

export interface FavoriteCategorySummary {
  category: string
  count: number
  avg_cook_time: number
  cooked_count: number
  never_cooked_count: number
  quick_dish_id: number
  quick_dish_name: string
}

export interface FavoriteOverviewStats {
  total: number
  category_total: number
  avg_cook_time: number
  cooked_count: number
  never_cooked_count: number
  quick_dish: Dish | null
  most_cooked_dish: Dish | null
}

export interface FavoriteOverview {
  items: FavoriteOverviewItem[]
  categories: FavoriteCategorySummary[]
  stats: FavoriteOverviewStats
  quick_picks: FavoriteOverviewItem[]
  most_cooked: FavoriteOverviewItem[]
  need_try: FavoriteOverviewItem[]
}

export interface DishRecordsStats {
  total_count: number
  lunch_count: number
  dinner_count: number
  yum_percent: number
  ok_percent: number
  no_percent: number
  avg_rating: number
  last_date: string
  avg_interval: number
}

export interface DishRecordsResponse {
  records: DishRecordWithDay[]
  stats: DishRecordsStats
}
