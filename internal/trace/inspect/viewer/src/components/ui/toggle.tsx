import { Checkbox } from "./checkbox";

type ToggleProps = {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
};

export function Toggle({ label, checked, onChange }: ToggleProps) {
  return (
    <label className="flex cursor-pointer items-center gap-2 rounded-lg border border-white/10 bg-white/[0.035] px-3 py-2 text-sm text-stone-300">
      <Checkbox checked={checked} onChange={(event) => onChange(event.target.checked)} />
      {label}
    </label>
  );
}
