import { useEffect, useState } from "react";

export function useLocalStorageBool(key: string, initialValue: boolean): [boolean, (value: boolean) => void] {
  const [value, setValue] = useState(() => {
    const stored = window.localStorage.getItem(key);
    if (stored === null) return initialValue;
    return stored === "true";
  });

  useEffect(() => {
    window.localStorage.setItem(key, String(value));
  }, [key, value]);

  return [value, setValue];
}

export function useLocalStorageNumber(key: string, initialValue: number): [number, (value: number) => void] {
  const [value, setValue] = useState(() => {
    const stored = window.localStorage.getItem(key);
    const parsed = Number(stored);
    return Number.isFinite(parsed) ? parsed : initialValue;
  });

  useEffect(() => {
    window.localStorage.setItem(key, String(value));
  }, [key, value]);

  return [value, setValue];
}
