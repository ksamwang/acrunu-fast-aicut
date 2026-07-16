export const roleLabels: Record<string, string> = { admin: "管理员", user: "用户" };
export const productStatusLabels: Record<string, string> = { active: "启用", archived: "已归档" };
export const assetStatusLabels: Record<string, string> = { active: "启用", archived: "已归档", uploaded: "已上传", ready: "可用" };
export const analysisStatusLabels: Record<string, string> = { pending_analysis: "待分析", analyzing: "分析中", ready: "已完成", failed: "失败" };
export const sourceTypeLabels: Record<string, string> = { visual_only: "纯画面", talking_head: "口播", "local-agent": "本地代理", "server-upload": "服务端上传", "manual-import": "手动导入" };
export const usabilityStatusLabels: Record<string, string> = { usable: "可用", needs_review: "待复核", discarded: "废弃" };
export const manualCleanStatusLabels: Record<string, string> = { cleaned: "已清洗" };
export const shotSizeLabels: Record<string, string> = { close_up: "特写", medium_close_up: "近景", medium_shot: "中景", full_shot: "全景", wide_shot: "远景" };
export const cameraMovementLabels: Record<string, string> = { static: "固定机位", pan: "水平摇镜", tilt: "垂直摇镜", push_in: "推进", pull_out: "拉远", tracking: "跟拍/平移", orbit: "环绕", zoom: "变焦", handheld: "手持", mixed: "复合运镜", unknown: "无法判断", slow_push_in: "推进" };
export const taskStatusLabels: Record<string, string> = { queued: "排队中", running: "执行中", completed: "已完成", failed: "失败" };
export const taskTypeLabels: Record<string, string> = { asset_analyze: "素材分析", asset_embedding: "素材向量化", asset_extract_frames: "素材抽帧", voiceover_generate: "生成旁白", edit_plan_generate: "生成编排", generation_render: "成品渲染", test: "测试任务" };

export function translateValue(value: string | undefined | null, labels: Record<string, string>) {
  return value ? labels[value] ?? value : "-";
}
