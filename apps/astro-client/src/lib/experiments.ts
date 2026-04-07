import { useState, useEffect, useCallback } from "react";

export interface Experiments {
  githubAutoBuild: boolean;
}

const STORAGE_KEY = "astro:experiments";

const DEFAULTS: Experiments = {
  githubAutoBuild: false,
};

function load(): Experiments {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULTS };
    return { ...DEFAULTS, ...JSON.parse(raw) };
  } catch {
    return { ...DEFAULTS };
  }
}

function save(experiments: Experiments) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(experiments));
}

export function useExperiments() {
  const [experiments, setExperimentsState] = useState<Experiments>(load);

  // Sync across tabs.
  useEffect(() => {
    function onStorage(e: StorageEvent) {
      if (e.key === STORAGE_KEY) setExperimentsState(load());
    }
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  const setExperiment = useCallback(<K extends keyof Experiments>(key: K, value: Experiments[K]) => {
    setExperimentsState((prev) => {
      const next = { ...prev, [key]: value };
      save(next);
      return next;
    });
  }, []);

  return { experiments, setExperiment };
}
