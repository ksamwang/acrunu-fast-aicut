import { useEffect, useMemo, useState } from "react";
import { Button, Checkbox, Empty, Input, Modal, Segmented, Select, Table, Tabs, Tag, Typography, Upload, message } from "antd";
import type { ColumnsType } from "antd/es/table";
import { FileDown, FileSpreadsheet, Upload as UploadIcon } from "lucide-react";
import {
  getScriptColumnOptions,
  mapScriptTable,
  maxImportedScriptRunes,
  maxWorkbenchScripts,
  parsePastedScripts,
  parseScriptFile,
  scriptImportTemplateContent,
  scriptImportTemplateFilename,
  scriptIdentity,
  type ImportedScript,
  type ParsedScriptFile
} from "./script-import";

type ImportMode = "append" | "replace";

type PreviewRow = ImportedScript & {
  key: string;
  index: number;
  rune_count: number;
  status: "ready" | "duplicate" | "too_long" | "overflow";
};

const statusLabels: Record<PreviewRow["status"], string> = {
  ready: "可导入",
  duplicate: "重复",
  too_long: "超过 4000 字",
  overflow: "超出 200 条"
};

export function ScriptImportModal({
  open,
  existingScripts,
  onCancel,
  onImport
}: {
  open: boolean;
  existingScripts: string[];
  onCancel: () => void;
  onImport: (scripts: ImportedScript[], mode: ImportMode) => void;
}) {
  const [source, setSource] = useState<"paste" | "file">("paste");
  const [mode, setMode] = useState<ImportMode>("append");
  const [pasteText, setPasteText] = useState("");
  const [parsedFile, setParsedFile] = useState<ParsedScriptFile | null>(null);
  const [fileName, setFileName] = useState("");
  const [parsingFile, setParsingFile] = useState(false);
  const [fileHasHeader, setFileHasHeader] = useState(false);
  const [scriptColumn, setScriptColumn] = useState(0);
  const [titleColumn, setTitleColumn] = useState<number | null>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    setSource("paste");
    setMode(existingScripts.length > 0 ? "append" : "replace");
    setPasteText("");
    setParsedFile(null);
    setFileName("");
  }, [existingScripts.length, open]);

  const fileScripts = useMemo(() => {
    if (!parsedFile) {
      return [];
    }
    if (parsedFile.kind === "text") {
      return parsedFile.scripts;
    }
    return mapScriptTable(parsedFile.table, {
      has_header: fileHasHeader,
      script_column: scriptColumn,
      title_column: titleColumn
    });
  }, [fileHasHeader, parsedFile, scriptColumn, titleColumn]);
  const fileTable = parsedFile?.kind === "table" ? parsedFile.table : null;
  const columnOptions = useMemo(
    () => fileTable ? getScriptColumnOptions(fileTable, fileHasHeader) : [],
    [fileHasHeader, fileTable]
  );
  const scripts = source === "paste" ? parsePastedScripts(pasteText) : fileScripts;
  const rows = useMemo(() => {
    const existing = new Set(mode === "append" ? existingScripts.map(scriptIdentity) : []);
    const seen = new Set<string>();
    const capacity = mode === "append" ? Math.max(0, maxWorkbenchScripts - existingScripts.length) : maxWorkbenchScripts;
    let readyCount = 0;
    return scripts.map<PreviewRow>((script, index) => {
      const identity = scriptIdentity(script.script_text);
      const runeCount = Array.from(script.script_text).length;
      let status: PreviewRow["status"] = "ready";
      if (existing.has(identity) || seen.has(identity)) {
        status = "duplicate";
      } else if (runeCount > maxImportedScriptRunes) {
        status = "too_long";
      } else if (readyCount >= capacity) {
        status = "overflow";
      } else {
        readyCount += 1;
      }
      seen.add(identity);
      return { ...script, key: `${index}-${identity}`, index: index + 1, rune_count: runeCount, status };
    });
  }, [existingScripts, mode, scripts]);
  const readyRows = rows.filter((row) => row.status === "ready");

  const columns: ColumnsType<PreviewRow> = [
    { title: "#", dataIndex: "index", width: 48 },
    { title: "标题", dataIndex: "title", width: 170, ellipsis: true },
    {
      title: "文案",
      dataIndex: "script_text",
      ellipsis: true,
      render: (value: string) => <span className="workbench-import-script-cell">{value}</span>
    },
    { title: "字数", dataIndex: "rune_count", width: 70 },
    {
      title: "状态",
      dataIndex: "status",
      width: 106,
      render: (value: PreviewRow["status"]) => <Tag color={value === "ready" ? "green" : value === "duplicate" ? "default" : "error"}>{statusLabels[value]}</Tag>
    }
  ];

  const loadFile = async (file: File) => {
    setParsingFile(true);
    try {
      const result = await parseScriptFile(file);
      setParsedFile(result);
      setFileName(file.name);
      let scriptCount = 0;
      if (result.kind === "table") {
        setFileHasHeader(result.table.has_header);
        setScriptColumn(result.table.script_column);
        setTitleColumn(result.table.title_column);
        scriptCount = mapScriptTable(result.table, result.table).length;
      } else {
        scriptCount = result.scripts.length;
      }
      if (scriptCount === 0) {
        message.warning("文件中没有可导入的文案");
      }
    } catch (error) {
      setParsedFile(null);
      setFileName("");
      message.error(error instanceof Error ? error.message : "文件解析失败");
    } finally {
      setParsingFile(false);
    }
    return false;
  };

  const downloadTemplate = () => {
    const url = URL.createObjectURL(new Blob([scriptImportTemplateContent], { type: "text/csv;charset=utf-8" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = scriptImportTemplateFilename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
  };

  return (
    <Modal
      open={open}
      width={820}
      centered
      className="workbench-import-modal"
      title="导入文案"
      okText={`导入 ${readyRows.length} 条`}
      cancelText="取消"
      okButtonProps={{ disabled: readyRows.length === 0 }}
      onCancel={onCancel}
      onOk={() => onImport(readyRows.map(({ title, script_text }) => ({ title, script_text })), mode)}
      destroyOnClose
    >
      <div className="workbench-import-toolbar">
        <Segmented<ImportMode>
          value={mode}
          options={[{ label: "追加", value: "append" }, { label: "替换", value: "replace" }]}
          onChange={setMode}
        />
        <Typography.Text type="secondary">当前 {existingScripts.length} 条 · 上限 {maxWorkbenchScripts} 条</Typography.Text>
      </div>
      <Tabs
        activeKey={source}
        onChange={(key) => setSource(key as "paste" | "file")}
        items={[
          {
            key: "paste",
            label: "批量粘贴",
            children: (
              <Input.TextArea
                data-testid="workbench-import-paste"
                value={pasteText}
                rows={7}
                placeholder="使用空白行分隔每条文案"
                onChange={(event) => setPasteText(event.target.value)}
              />
            )
          },
          {
            key: "file",
            label: "文件导入",
            children: (
              <div className="workbench-import-file-panel">
                <div className="workbench-import-file-row">
                  <Upload accept=".txt,.csv,.xlsx" showUploadList={false} beforeUpload={loadFile}>
                    <Button icon={<UploadIcon size={16} />} loading={parsingFile}>选择文件</Button>
                  </Upload>
                  {fileName ? <span><FileSpreadsheet size={15} />{fileName}</span> : <Typography.Text type="secondary">TXT / CSV / XLSX</Typography.Text>}
                  <Button className="workbench-import-template" type="link" icon={<FileDown size={15} />} onClick={downloadTemplate}>
                    下载模板
                  </Button>
                </div>
                {fileTable ? (
                  <div className="workbench-import-mapping">
                    <Checkbox
                      data-testid="workbench-import-header-row"
                      checked={fileHasHeader}
                      onChange={(event) => setFileHasHeader(event.target.checked)}
                    >
                      首行为列名
                    </Checkbox>
                    <label>
                      <span>文案列</span>
                      <Select
                        data-testid="workbench-import-script-column"
                        value={scriptColumn}
                        options={columnOptions}
                        onChange={(value) => {
                          setScriptColumn(value);
                          if (titleColumn === value) {
                            setTitleColumn(null);
                          }
                        }}
                      />
                    </label>
                    <label>
                      <span>标题列</span>
                      <Select
                        data-testid="workbench-import-title-column"
                        value={titleColumn ?? -1}
                        options={[
                          { label: "不使用标题列", value: -1 },
                          ...columnOptions.filter((option) => option.value !== scriptColumn)
                        ]}
                        onChange={(value) => setTitleColumn(value === -1 ? null : value)}
                      />
                    </label>
                  </div>
                ) : null}
              </div>
            )
          }
        ]}
      />
      <div className="workbench-import-preview" data-testid="workbench-import-preview">
        {rows.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可预览文案" />
        ) : (
          <Table<PreviewRow>
            size="small"
            columns={columns}
            dataSource={rows}
            pagination={false}
            scroll={{ y: 260 }}
          />
        )}
      </div>
      <div className="workbench-import-summary">
        <span>识别 {rows.length} 条</span>
        <span>可导入 {readyRows.length} 条</span>
        {rows.length !== readyRows.length ? <span>跳过 {rows.length - readyRows.length} 条</span> : null}
      </div>
    </Modal>
  );
}
