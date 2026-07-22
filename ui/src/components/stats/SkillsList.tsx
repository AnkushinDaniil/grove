import { Sparkles } from "lucide-react";
import { MiniBarList } from "./charts/MiniBarList";
import { formatTokens } from "../../lib/usageFormat";
import type { StatsSkill } from "../../gen/types";

interface SkillsListProps {
  skills: StatsSkill[];
}

export function SkillsList({ skills }: SkillsListProps) {
  if (skills.length === 0) return null;
  const sorted = [...skills].sort((a, b) => b.invocations - a.invocations);

  return (
    <div>
      <h3 className="mb-1.5 flex items-center gap-1.5 font-sans text-2xs font-medium text-ink-faint">
        <Sparkles size={12} />
        Skills
      </h3>
      <MiniBarList items={sorted.map((s) => ({ key: s.skill, label: s.skill, value: s.invocations }))} formatValue={formatTokens} />
    </div>
  );
}
