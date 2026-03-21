import * as React from "react"

import { cn } from "@/lib/utils"
import { inputBase, inputFocusVisible, inputInvalid } from "./input"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "placeholder:text-faint-foreground selection:bg-primary selection:text-primary-foreground w-full min-w-0 min-h-[80px] py-2.5 resize-none",
        inputBase,
        inputFocusVisible,
        inputInvalid,
        className
      )}
      {...props}
    />
  )
}

export { Textarea }
