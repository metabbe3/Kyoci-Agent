import { type ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * Marquee — pure-CSS infinite ticker. Renders children twice back-to-back
 * so the loop is seamless. The keyframe `ticker` translates the row by
 * -50% (one full copy) over `durationSec` seconds. Pauses on hover.
 *
 * Use for status tickers, partner logos, activity feeds. Keep content
 * light per copy or it'll choke low-end devices.
 */
export function Marquee({
  children,
  durationSec = 30,
  className,
  reverse = false,
}: {
  children: ReactNode;
  durationSec?: number;
  className?: string;
  reverse?: boolean;
}) {
  return (
    <div
      className={cn("relative flex overflow-hidden", className)}
      aria-hidden
    >
      <div
        className="flex shrink-0 items-center gap-8 group hover:[animation-play-state:paused]"
        style={{
          animation: `ticker ${durationSec}s linear infinite`,
          animationDirection: reverse ? "reverse" : "normal",
          paddingRight: "2rem",
        }}
      >
        {children}
      </div>
      <div
        className="flex shrink-0 items-center gap-8 group hover:[animation-play-state:paused]"
        style={{
          animation: `ticker ${durationSec}s linear infinite`,
          animationDirection: reverse ? "reverse" : "normal",
          paddingRight: "2rem",
        }}
      >
        {children}
      </div>
    </div>
  );
}
