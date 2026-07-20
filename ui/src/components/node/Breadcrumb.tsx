import { Link } from "react-router";
import { ChevronRight } from "lucide-react";
import { useNodePath } from "../../hooks/useNodePath";
import type { NodeID } from "../../gen/types";

interface BreadcrumbProps {
  nodeId: NodeID;
}

export function Breadcrumb({ nodeId }: BreadcrumbProps) {
  const path = useNodePath(nodeId);
  if (path.length === 0) return null;

  return (
    <nav aria-label="Breadcrumb" className="flex items-center gap-1 overflow-x-auto text-2xs text-ink-faint">
      {path.map((node, i) => {
        const isLast = i === path.length - 1;
        return (
          <span key={node.id} className="flex shrink-0 items-center gap-1">
            {i > 0 && <ChevronRight size={11} className="shrink-0 text-ink-disabled" />}
            {isLast ? (
              <span className="text-ink-muted">{node.title}</span>
            ) : (
              <Link to={`/n/${node.id}`} className="underline-offset-2 hover:text-accent hover:underline">
                {node.title}
              </Link>
            )}
          </span>
        );
      })}
    </nav>
  );
}
