export const maxWorkbenchScripts = 200;
export const maxImportedScriptRunes = 4000;

export type ImportedScript = {
  title: string;
  script_text: string;
};

export type ScriptFileTable = {
  rows: string[][];
  has_header: boolean;
  script_column: number;
  title_column: number | null;
};

export type ParsedScriptFile =
  | { kind: "text"; scripts: ImportedScript[] }
  | { kind: "table"; table: ScriptFileTable };

export type ScriptColumnOption = {
  label: string;
  value: number;
};

export const scriptImportTemplateFilename = "文案批量导入模板.csv";
export const scriptImportTemplateContent = "\uFEFF标题,文案\r\n";

const scriptHeaders = new Set(["script_text", "script", "content", "文案", "正文", "口播文案"]);
const titleHeaders = new Set(["title", "hook", "标题", "文案标题"]);

function normalizedHeader(value: string) {
  return value.replace(/^\uFEFF/, "").trim().toLocaleLowerCase().replace(/[\s-]+/g, "_");
}

export function scriptIdentity(text: string) {
  return text.trim().replace(/\s+/g, "");
}

export function deriveScriptHook(text: string, title = "") {
  const explicitTitle = title.trim();
  if (explicitTitle) {
    return explicitTitle;
  }
  const normalized = text.trim();
  const sentence = normalized.match(/^([\s\S]*?[。！？!?])/u)?.[1] ?? normalized.split(/\r?\n/, 1)[0] ?? normalized;
  return Array.from(sentence.trim()).slice(0, 60).join("");
}

export function parsePastedScripts(value: string): ImportedScript[] {
  return value
    .replace(/\r\n?/g, "\n")
    .trim()
    .split(/\n[\t ]*\n+/)
    .map((scriptText) => scriptText.trim())
    .filter(Boolean)
    .map((scriptText) => ({ title: deriveScriptHook(scriptText), script_text: scriptText }));
}

function normalizeTableRows(input: unknown[][]) {
  return input
    .map((row) => row.map((cell) => String(cell ?? "").trim()))
    .filter((row) => row.some(Boolean));
}

function tableFromRows(input: unknown[][]): ScriptFileTable {
  const rows = normalizeTableRows(input);
  if (rows.length === 0) {
    return { rows: [], has_header: false, script_column: 0, title_column: null };
  }

  const headers = rows[0].map(normalizedHeader);
  const detectedScriptColumn = headers.findIndex((header) => scriptHeaders.has(header));
  const detectedTitleColumn = headers.findIndex((header) => titleHeaders.has(header));
  const hasHeader = detectedScriptColumn >= 0 || detectedTitleColumn >= 0;
  const columnCount = Math.max(...rows.map((row) => row.length));
  let scriptColumn = detectedScriptColumn;
  if (scriptColumn < 0) {
    scriptColumn = columnCount > 1 ? 1 : 0;
    if (scriptColumn === detectedTitleColumn) {
      scriptColumn = Array.from({ length: columnCount }, (_, index) => index).find((index) => index !== detectedTitleColumn) ?? 0;
    }
  }
  const titleColumn = detectedTitleColumn >= 0 ? detectedTitleColumn : (!hasHeader && scriptColumn === 1 ? 0 : null);
  return { rows, has_header: hasHeader, script_column: scriptColumn, title_column: titleColumn };
}

export function getScriptColumnOptions(table: ScriptFileTable, hasHeader: boolean): ScriptColumnOption[] {
  const columnCount = table.rows.length > 0 ? Math.max(...table.rows.map((row) => row.length)) : 0;
  return Array.from({ length: columnCount }, (_, index) => {
    const header = hasHeader ? (table.rows[0]?.[index] ?? "").trim() : "";
    const sampleStart = hasHeader ? 1 : 0;
    const sample = table.rows.slice(sampleStart).map((row) => row[index]?.trim()).find(Boolean) ?? "";
    const name = header || `第 ${index + 1} 列`;
    const hint = sample ? ` · ${Array.from(sample).slice(0, 18).join("")}` : "";
    return { value: index, label: `${name}${hint}` };
  });
}

export function mapScriptTable(
  table: ScriptFileTable,
  mapping: { has_header: boolean; script_column: number; title_column: number | null }
): ImportedScript[] {
  const dataRows = mapping.has_header ? table.rows.slice(1) : table.rows;

  return dataRows.flatMap((row) => {
    const scriptText = (row[mapping.script_column] ?? "").trim();
    if (!scriptText) {
      return [];
    }
    const title = mapping.title_column !== null ? (row[mapping.title_column] ?? "").trim() : "";
    return [{ title: deriveScriptHook(scriptText, title), script_text: scriptText }];
  });
}

export async function parseScriptFile(file: File): Promise<ParsedScriptFile> {
  const extension = file.name.split(".").pop()?.toLocaleLowerCase();
  if (extension === "txt") {
    return { kind: "text", scripts: parsePastedScripts(await file.text()) };
  }
  if (extension === "csv") {
    const [{ default: Papa }, text] = await Promise.all([import("papaparse"), file.text()]);
    const result = Papa.parse<string[]>(text, { skipEmptyLines: "greedy" });
    if (result.errors.length > 0) {
      throw new Error(`CSV 第 ${(result.errors[0].row ?? 0) + 1} 行格式不正确`);
    }
    return { kind: "table", table: tableFromRows(result.data) };
  }
  if (extension === "xlsx") {
    const { default: readXlsxFile } = await import("read-excel-file/browser");
    const rows = await readXlsxFile(file);
    return { kind: "table", table: tableFromRows(rows as unknown as unknown[][]) };
  }
  throw new Error("仅支持 TXT、CSV 或 XLSX 文件");
}
