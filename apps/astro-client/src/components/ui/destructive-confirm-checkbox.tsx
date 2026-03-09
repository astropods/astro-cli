interface DestructiveConfirmCheckboxProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  children: React.ReactNode;
}

export function DestructiveConfirmCheckbox({
  checked,
  onChange,
  children,
}: DestructiveConfirmCheckboxProps) {
  return (
    <label className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 p-3 cursor-pointer">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="mt-0.5 accent-destructive"
      />
      <span className="text-[13px] leading-snug text-destructive">
        {children}
      </span>
    </label>
  );
}
