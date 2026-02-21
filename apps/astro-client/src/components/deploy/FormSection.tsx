import type { ReactNode } from "react";

export interface FormSectionProps {
  title: string;
  description: string;
  children: ReactNode;
}

export function FormSection({ title, description, children }: FormSectionProps) {
  return (
    <section>
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-foreground">{title}</h2>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
      {children}
    </section>
  );
}
