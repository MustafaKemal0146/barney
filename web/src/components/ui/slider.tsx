import { cn } from "cn"

/** Yerel input[type=range]; 0..max tamsayı kaydırıcı. */
function Slider({
  value, onChange, min = 0, max = 10, className, label,
}: { value: number; onChange: (v: number) => void; min?: number; max?: number; className?: string; label: string }) {
  const pct = ((value - min) / (max - min)) * 100
  return (
    <input
      type="range"
      aria-label={label}
      min={min}
      max={max}
      step={1}
      value={value}
      onChange={(e) => onChange(Number(e.target.value))}
      className={cn("bslider", className)}
      style={{ ['--pct' as string]: `${pct}%` }}
    />
  )
}
export { Slider }
