import { useEffect, useMemo, useState } from "react";

interface Container {
  name: string;
  ready: boolean;
  vars: { key: string; value: string; secret: boolean; source: string }[];
}

export function useContainerSelection(containers: Container[]) {
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
