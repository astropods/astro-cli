import { useState, useEffect, useRef } from "react";

export function useSavedFlash() {
  const [showSaved, setShowSaved] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => () => clearTimeout(timerRef.current), []);

  const flash = () => {
    setShowSaved(true);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setShowSaved(false), 2000);
  };

  return { showSaved, flash };
}
