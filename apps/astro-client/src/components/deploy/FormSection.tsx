import type { ReactNode } from "react";

export interface FormSectionProps {
  title: string;
  description: string;
  children: ReactNode;
  /** Optional action element rendered to the right of the title. */
  action?: ReactNode;
}

export function FormSection({ title, description, children, action }: FormSectionProps) {
  return (
    <section>
      <div className="mb-1.5">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-foreground">{title}</h2>
          {action}
        </div>
        <p className="text-[13px] text-muted-foreground mt-0.5">{description}</p>
      </div>
      <hr className="border-border-strong mb-5 mt-4" />
      {children}
    </section>
  );
}
