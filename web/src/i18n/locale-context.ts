import { createContext } from "react";
import type { Locale, MessageKey } from "./messages";

export type LocaleApi = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey) => string;
};

export const LocaleContext = createContext<LocaleApi | null>(null);
