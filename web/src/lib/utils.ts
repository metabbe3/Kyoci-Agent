import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatBytes(gb: number): string {
  if (gb < 1) return `${Math.round(gb * 1024)} MB`;
  return `${gb} GB`;
}
