import type { Dish } from "@/types"
import { asArray } from "@/lib/utils"

const categoryEmojis: Record<string, string> = {
  川菜: "🌶", 粤菜: "🐟", 家常菜: "🍳", 快手菜: "⚡",
  汤品: "🍲", 主食: "🍚", 小食: "🥟",
}

export function getDishEmoji(d: Dish): string {
  return categoryEmojis[d.category] || d.image_url || "🍽"
}

export function isImageUrl(url: string | undefined): boolean {
  if (!url) return false
  return url.startsWith("/uploads/") || url.startsWith("http") || /\.(jpg|jpeg|png|webp|gif|svg)(\?|$)/i.test(url)
}

export function getDishImageUrl(d: Dish): string | null {
  if (isImageUrl(d.image_url)) return d.image_url
  const arr = asArray<string>(d.images)
  if (arr.length > 0 && typeof arr[0] === "string" && isImageUrl(arr[0])) return arr[0]
  return null
}

interface DishImageProps {
  dish: Dish
  className?: string
  imgClassName?: string
  emojiSize?: string
}

export default function DishImage({ dish, className, imgClassName, emojiSize = "text-4xl" }: DishImageProps) {
  const url = getDishImageUrl(dish)
  if (url) {
    return (
      <div className={className}>
        <img src={url} alt={dish.name} loading="lazy" className={`w-full h-full object-cover ${imgClassName || ""}`} />
      </div>
    )
  }
  return (
    <div className={`${className} flex items-center justify-center ${emojiSize}`}>
      {getDishEmoji(dish)}
    </div>
  )
}
