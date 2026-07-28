import { useMemo, useState } from "react";
import { Check, PencilLine, ShieldAlert, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";

import { InteractionField } from "./InteractionField";
import { describeFields, humanize, initialFormValue, missingRequired } from "./schema";
import type { FormValue, Interaction, InteractionAction } from "./types";

export interface InteractionFormProps {
  interaction: Interaction;
  pending?: boolean;
  /** Server-side validation or delivery error to surface inline. */
  error?: string;
  onSubmit: (content: FormValue) => void;
  onDecline?: () => void;
  onCancel?: () => void;
  onRespond?: (text: string) => void;
}

export function InteractionForm({
  interaction,
  pending,
  error,
  onSubmit,
  onDecline,
  onCancel,
  onRespond,
}: InteractionFormProps) {
  const fields = useMemo(() => describeFields(interaction.dataSchema), [interaction.dataSchema]);
  const [value, setValue] = useState<FormValue>(() => initialFormValue(fields, interaction.value));
  const [showErrors, setShowErrors] = useState(false);
  const [respondMode, setRespondMode] = useState(false);
  const [respondText, setRespondText] = useState("");

  const allows = (a: InteractionAction) => interaction.actions.includes(a);
  const isPermission = interaction.intent === "tool_permission";
  const missing = missingRequired(fields, value);

  const setField = (key: string, v: unknown) => setValue((prev) => ({ ...prev, [key]: v }));

  const submit = () => {
    if (missing.length > 0) {
      setShowErrors(true);
      return;
    }
    onSubmit(value);
  };

  const sendRespond = () => {
    const text = respondText.trim();
    if (text === "") {
      setShowErrors(true);
      return;
    }
    onRespond?.(text);
  };

  const enterRespondMode = () => {
    setShowErrors(false);
    setRespondMode(true);
  };

  if (respondMode) {
    return (
      <Card className="flex flex-col gap-3 p-4">
        <Header interaction={interaction} />
        <Textarea
          autoFocus
          value={respondText}
          disabled={pending}
          placeholder="Type your answer…"
          onChange={(e) => setRespondText(e.target.value)}
        />
        {showErrors && respondText.trim() === "" && <ErrorText>Enter a reply.</ErrorText>}
        {error && <ErrorText>{error}</ErrorText>}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" disabled={pending} onClick={() => setRespondMode(false)}>
            Back
          </Button>
          <Button disabled={pending} onClick={sendRespond}>
            Send
          </Button>
        </div>
      </Card>
    );
  }

  return (
    <Card className="flex flex-col gap-4 p-4">
      <Header interaction={interaction} />

      {fields.length > 0 && (
        <div className="flex flex-col gap-4">
          {fields.map((field) => (
            <div key={field.key}>
              <InteractionField
                field={field}
                value={value[field.key]}
                disabled={pending}
                invalid={showErrors && missing.includes(field.key)}
                onChange={(v) => setField(field.key, v)}
              />
              {showErrors && missing.includes(field.key) && <ErrorText>Required.</ErrorText>}
            </div>
          ))}
        </div>
      )}

      {error && <ErrorText>{error}</ErrorText>}

      <div className="flex flex-wrap items-center justify-end gap-2">
        {allows("respond") && onRespond && (
          <Button variant="link" className="mr-auto px-0" disabled={pending} onClick={enterRespondMode}>
            <PencilLine />
            Write your own reply
          </Button>
        )}
        {allows("cancel") && onCancel && (
          <Button variant="ghost" disabled={pending} onClick={onCancel}>
            Cancel
          </Button>
        )}
        {allows("decline") && onDecline && (
          <Button variant="outline" disabled={pending} onClick={onDecline}>
            <X />
            {isPermission ? "Deny" : "Decline"}
          </Button>
        )}
        {allows("submit") && (
          <Button disabled={pending} onClick={submit}>
            <Check />
            {isPermission ? "Approve" : "Submit"}
          </Button>
        )}
      </div>
    </Card>
  );
}

// tool_permission renders as a gate: humanized tool name as heading, optional
// agent `message` as subtext. schema.description is LLM-facing, so never shown.
function Header({ interaction }: { interaction: Interaction }) {
  const message = interaction.message?.trim();

  if (interaction.intent === "tool_permission") {
    const title = interaction.dataSchema.title;
    const heading = title ? humanize(title) : undefined;
    return (
      <div className="flex flex-col gap-1.5">
        <span className="flex items-center gap-1.5 text-label text-faint-foreground">
          <ShieldAlert className="size-3.5" />
          Permission required
        </span>
        {heading && <p className="text-body whitespace-pre-wrap text-foreground">{heading}</p>}
        {message && <p className="text-body-sm text-muted-foreground">{message}</p>}
      </div>
    );
  }
  return message ? <p className="text-body whitespace-pre-wrap text-foreground">{message}</p> : null;
}

function ErrorText({ children }: { children: React.ReactNode }) {
  return <p className="text-xs text-destructive">{children}</p>;
}
