import { useEffect, useRef, useState } from "react";

function scrollParent(el: Element | null): Element | null {
  for (let p = el?.parentElement; p; p = p.parentElement) {
    const overflowY = getComputedStyle(p).overflowY;
    if (overflowY === "auto" || overflowY === "scroll") return p;
  }
  return null;
}

export function useInViewport<T extends Element>(rootMargin = "400px") {
  const ref = useRef<T | null>(null);
  const [inViewport, setInViewport] = useState(
    () => typeof IntersectionObserver === "undefined",
  );

  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") return;
    const el = ref.current;
    if (!el) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[entries.length - 1];
        if (entry) setInViewport(entry.isIntersecting);
      },
      { root: scrollParent(el), rootMargin },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [rootMargin]);

  return { ref, inViewport };
}
