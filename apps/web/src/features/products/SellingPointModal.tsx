import { Form, Input, InputNumber, Modal, type FormInstance } from "antd";
import type { Product, SellingPoint } from "../../shared/types/product";

type SellingPointModalProps = {
  open: boolean;
  editingSellingPoint: SellingPoint | null;
  selectedProduct: Product | null;
  form: FormInstance;
  onSave: () => void;
  onCancel: () => void;
};

export function SellingPointModal({ open, editingSellingPoint, selectedProduct, form, onSave, onCancel }: SellingPointModalProps) {
  return (
    <Modal
      title={editingSellingPoint ? "编辑卖点" : selectedProduct ? `新建卖点：${selectedProduct.name}` : "新建卖点"}
      open={open}
      onOk={onSave}
      onCancel={onCancel}
      okText="确认"
      cancelText="取消"
    >
      <Form form={form} layout="vertical">
        <Form.Item name="title" label="标题" rules={[{ required: true, message: "请输入卖点标题" }]}>
          <Input />
        </Form.Item>
        <Form.Item name="priority" label="优先级" initialValue={0}>
          <InputNumber min={0} />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea rows={3} />
        </Form.Item>
      </Form>
    </Modal>
  );
}
