import { useContext } from "react";
import { LocaleContext, type LocaleApi } from "./locale-context";

export function useLocale(): LocaleApi {
  const ctx = useContext(LocaleContext);
  if (!ctx) throw new Error("useLocale requires LocaleProvider");
  return ctx;
}
