import { Button, Form, Input, Modal, Space, Typography, type FormInstance } from "antd";
import type { Product } from "../../shared/types/product";
import { readImageFileAsDataURL } from "./product-reference";

type ProductEditorModalProps = {
  open: boolean;
  editingProduct: Product | null;
  form: FormInstance;
  onSave: () => void;
  onCancel: () => void;
  onError: (message: string) => void;
};

export function ProductEditorModal({ open, editingProduct, form, onSave, onCancel, onError }: ProductEditorModalProps) {
  return (
    <Modal title={editingProduct ? "编辑产品" : "新建产品"} open={open} onOk={onSave} onCancel={onCancel} okText="确认" cancelText="取消">
      <Form form={form} layout="vertical">
        <Form.Item name="name" label="产品名称" rules={[{ required: true, message: "请输入产品名称" }]}>
          <Input />
        </Form.Item>
        <Form.Item name="category" label="分类">
          <Input />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea rows={3} />
        </Form.Item>
        <Form.Item name="reference_image" label="产品白底参考图">
          <Input type="hidden" />
        </Form.Item>
        <Form.Item shouldUpdate noStyle>
          {() => {
            const referenceImage = form.getFieldValue("reference_image");
            return (
              <Space direction="vertical" className="wide-space">
                {referenceImage ? (
                  <img className="product-reference-preview" src={referenceImage} alt="产品白底参考图预览" />
                ) : (
                  <Typography.Text type="secondary">可上传一张白底产品图，用于 VLM 标注时辅助识别产品。</Typography.Text>
                )}
                <Space wrap>
                  <input
                    type="file"
                    accept="image/png,image/jpeg,image/webp"
                    onChange={(event) => {
                      const file = event.target.files?.[0];
                      event.currentTarget.value = "";
                      if (!file) {
                        return;
                      }
                      void readImageFileAsDataURL(file)
                        .then((dataURL) => form.setFieldValue("reference_image", dataURL))
                        .catch((error) => onError(error instanceof Error ? error.message : "读取参考图失败"));
                    }}
                  />
                  <Button size="small" disabled={!referenceImage} onClick={() => form.setFieldValue("reference_image", "")}>移除参考图</Button>
                </Space>
              </Space>
            );
          }}
        </Form.Item>
      </Form>
    </Modal>
  );
}
