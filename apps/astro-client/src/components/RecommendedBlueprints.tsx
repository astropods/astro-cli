import { BlueprintCard, type BlueprintCardProps } from "@/components/BlueprintCard";

export type RecommendedBlueprint = BlueprintCardProps;

export interface RecommendedBlueprintsProps {
  blueprints: RecommendedBlueprint[];
}

export function RecommendedBlueprints({ blueprints }: RecommendedBlueprintsProps) {
  if (blueprints.length === 0) return null;

  return (
    <section className="@container mt-8 pt-8 border-t border-border">
      <h2 className="text-lg font-semibold mb-4">Recommended Blueprints</h2>
      <div className="grid grid-cols-1 @[480px]:grid-cols-2 gap-4">
        {blueprints.map((blueprint) => (
          <BlueprintCard key={blueprint.slug} {...blueprint} />
        ))}
      </div>
    </section>
  );
}
