export function subtitleDisplayText(text: string) {
  return text.replace(/\p{P}+/gu, "").replace(/\s+/g, " ").trim();
}
