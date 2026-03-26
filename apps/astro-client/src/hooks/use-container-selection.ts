import { useEffect, useMemo, useState } from "react";
import type { MappedContainer } from "@/components/deployed-agent/detail/deployments/history/types";

export function useContainerSelection(containers: MappedContainer[]) {
  const [selectedContainer, setSelectedContainer] = useState<string>(containers[0]?.name ?? "");

  useEffect(() => {
    setSelectedContainer((prev) => {
      if (containers.length === 0) return "";
      if (containers.some((c) => c.name === prev)) return prev;
      return containers[0].name;
    });
  }, [containers]);

  const activeContainer = useMemo(
    () => containers.find((c) => c.name === selectedContainer) ?? containers[0],
    [containers, selectedContainer],
  );

  return { selectedContainer, setSelectedContainer, activeContainer };
}
