import { SegmentedControl } from "../common/SegmentedControl";
import type { StatsRange } from "../../gen/types";

const OPTIONS: { value: StatsRange; label: string }[] = [
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
  { value: "30d", label: "30d" },
];

interface RangeSwitcherProps {
  value: StatsRange;
  onChange: (range: StatsRange) => void;
}

export function RangeSwitcher({ value, onChange }: RangeSwitcherProps) {
  return <SegmentedControl options={OPTIONS} value={value} onChange={onChange} />;
}
