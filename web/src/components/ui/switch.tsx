import * as React from "react";
import * as SwitchPrimitive from "@radix-ui/react-switch";
import { cn } from "@/lib/utils";

export const Switch = React.forwardRef<
  React.ElementRef<typeof SwitchPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof SwitchPrimitive.Root>
>(({ className, ...props }, ref) => (
  <SwitchPrimitive.Root
    ref={ref}
    className={cn(
      "peer inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-all duration-300",
      "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-lime)]/40 focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-void)]",
      "disabled:cursor-not-allowed disabled:opacity-50",
      "data-[state=checked]:bg-[var(--color-lime)] data-[state=checked]:shadow-[0_0_18px_-4px_rgba(198,244,50,0.7)]",
      "data-[state=unchecked]:bg-white/10",
      className
    )}
    {...props}
  >
    <SwitchPrimitive.Thumb
      className={cn(
        "pointer-events-none block h-4 w-4 rounded-full bg-white shadow-lg ring-0 transition-transform duration-300",
        "data-[state=checked]:translate-x-5 data-[state=unchecked]:translate-x-1"
      )}
    />
  </SwitchPrimitive.Root>
));
Switch.displayName = "Switch";
