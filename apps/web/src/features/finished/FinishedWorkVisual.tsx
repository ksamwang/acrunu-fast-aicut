import { Clapperboard } from "lucide-react";
import type { FinishedWork } from "../../shared/types/generation";

export function FinishedWorkVisual({ work, compact = false }: { work: FinishedWork; compact?: boolean }) {
  return work.product_cover_url ? (
    <img src={work.product_cover_url} alt={`${work.product_name}成品预览`} />
  ) : (
    <div className={`finished-work-fallback${compact ? " is-compact" : ""}${work.is_demo ? " is-demo" : ""}`}>
      <Clapperboard size={compact ? 26 : 36} />
      <span>{work.product_name}</span>
    </div>
  );
}
