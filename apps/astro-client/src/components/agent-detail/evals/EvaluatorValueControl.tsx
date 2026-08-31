import type { ReactNode } from "react";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { EvaluatorOutput } from "@/lib/api";
import { formatEvaluatorValue } from "./evaluator-values";

const CONTROL_WIDTH = "w-40";
const BOOLEAN_OPTIONS = [true, false];

interface EvaluatorValueControlProps {
  output: EvaluatorOutput;
  label: string;
  value: unknown;
  disabled?: boolean;
  className?: string;
  controlRef?: React.Ref<HTMLButtonElement>;
  onChange: (value: unknown) => void;
}

export function EvaluatorValueControl({
  output,
  ...control
}: EvaluatorValueControlProps) {
  switch (output.type) {
    case "boolean":
      return <ValueSelect {...control} options={BOOLEAN_OPTIONS} />;
    case "enum":
      return <ValueSelect {...control} options={output.options ?? []} />;
    case "number":
      return <ValueInput {...control} output={output} inputType="number" />;
    case "string":
      return <ValueInput {...control} output={output} inputType="text" />;
  }
}

type ControlProps = Omit<EvaluatorValueControlProps, "output">;

function ValueSelect({
  label,
  value,
  disabled,
  className,
  controlRef,
  onChange,
  options,
}: ControlProps & { options: unknown[] }) {
  const text = String(value ?? "");

  return (
    <ControlBox className={className}>
      <Select
        value={text}
        disabled={disabled}
        onValueChange={(next) => onChange(optionForValue(next, options))}
      >
        <SelectTrigger
          ref={controlRef}
          aria-label={label}
          className="h-7 w-full text-body-sm"
          onClear={text && !disabled ? () => onChange(undefined) : undefined}
        >
          <SelectValue placeholder="Select" />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={String(option)} value={String(option)}>
              {formatEvaluatorValue(option)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </ControlBox>
  );
}

function ValueInput({
  label,
  value,
  disabled,
  className,
  onChange,
  output,
  inputType,
}: ControlProps & { output: EvaluatorOutput; inputType: "number" | "text" }) {
  return (
    <ControlBox className={className}>
      <Input
        type={inputType}
        value={String(value ?? "")}
        disabled={disabled}
        aria-label={label}
        min={output.minimum}
        max={output.maximum}
        maxLength={output.max_length}
        onChange={(event) => onChange(parseInput(event.target.value, inputType))}
        className="h-7 w-full text-body-sm"
      />
    </ControlBox>
  );
}

// Sizes the box, not the trigger: the select's clear-button wrapper stretches.
function ControlBox({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cn("flex-none", CONTROL_WIDTH, className)}>{children}</div>
  );
}

function optionForValue(raw: string, options: unknown[]) {
  return options.find((option) => String(option) === raw);
}

function parseInput(raw: string, inputType: "number" | "text") {
  if (raw === "") {
    return undefined;
  }
  if (inputType === "text") {
    return raw;
  }
  const parsed = Number(raw);
  return Number.isNaN(parsed) ? undefined : parsed;
}
