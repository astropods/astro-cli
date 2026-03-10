import type { ReactNode } from "react";

export interface FormSectionProps {
  title: string;
  description: string;
  children: ReactNode;
}

export function FormSection({ title, description, children }: FormSectionProps) {
  return (
    <section>
      <div className="mb-1.5">
        <h2 className="text-base font-bold text-foreground">{title}</h2>
        <p className="text-[13px] text-faint-foreground mt-0.5">{description}</p>
      </div>
      <hr className="border-border-strong mb-5 mt-4" />
      {children}
    </section>
  );
}
