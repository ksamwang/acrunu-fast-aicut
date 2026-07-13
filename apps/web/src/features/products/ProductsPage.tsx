import { useEffect, useState } from "react";
import { Button, Card, Drawer, Empty, Form, Input, InputNumber, Modal, Popconfirm, Space, Table, Tag, Typography, message } from "antd";
import { useResource } from "../../shared/hooks/use-resource";
import { assetDisplayTitle, assetVideoURL } from "../../shared/lib/asset-display";
import { formatDuration } from "../../shared/lib/format";
import { analysisStatusLabels, assetStatusLabels, cameraMovementLabels, productStatusLabels, shotSizeLabels, sourceTypeLabels, translateValue } from "../../shared/lib/labels";
import type { Asset } from "../../shared/types/asset";
import type { Product, ProductStats, SellingPoint } from "../../shared/types/product";
import { productReferenceImage, readImageFileAsDataURL } from "./product-reference";
import { deleteProduct as removeProduct, deleteSellingPoint as removeSellingPoint, getProductStats, listProducts, listSellingPointAssets, listSellingPoints, saveProduct as persistProduct, saveSellingPoint as persistSellingPoint } from "./api";
import { ProductEditorModal } from "./ProductEditorModal";
import "./styles.css";

export function ProductManagementPage({ token }: { token: string }) {
  const products = useResource<Product[]>("/api/products", token, [], listProducts);
  const [selectedProductID, setSelectedProductID] = useState<string | null>(null);
  const [selectedSellingPointID, setSelectedSellingPointID] = useState<string | null>(null);
  const [productStatsMap, setProductStatsMap] = useState<Record<string, ProductStats>>({});
  const [productStatsLoading, setProductStatsLoading] = useState(false);
  const [productOpen, setProductOpen] = useState(false);
  const [sellingPointOpen, setSellingPointOpen] = useState(false);
  const [sellingPointManagerOpen, setSellingPointManagerOpen] = useState(false);
  const [sellingPointAssetsOpen, setSellingPointAssetsOpen] = useState(false);
  const [editingProduct, setEditingProduct] = useState<Product | null>(null);
  const [editingSellingPoint, setEditingSellingPoint] = useState<SellingPoint | null>(null);
  const [productForm] = Form.useForm();
  const [sellingPointForm] = Form.useForm();

  const sellingPoints = useResource<SellingPoint[]>(
    selectedProductID ? `/api/products/${selectedProductID}/selling-points` : null,
    token,
    [selectedProductID],
    listSellingPoints
  );
  const sellingPointAssets = useResource<Asset[]>(
    selectedSellingPointID ? `/api/selling-points/${selectedSellingPointID}/assets` : null,
    token,
    [selectedSellingPointID],
    listSellingPointAssets
  );

  useEffect(() => {
    const productList = products.data ?? [];
    if (productList.length === 0) {
      setProductStatsMap({});
      setProductStatsLoading(false);
      return;
    }

    let cancelled = false;
    setProductStatsLoading(true);
    void Promise.all(
      productList.map(async (product) => {
        const stats = await getProductStats(product.id, token);
        return [product.id, stats] as const;
      })
    )
      .then((entries) => {
        if (!cancelled) {
          setProductStatsMap(Object.fromEntries(entries));
        }
      })
      .catch((error) => {
        if (!cancelled) {
          message.error(error instanceof Error ? error.message : "加载产品统计失败");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setProductStatsLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [products.data, token]);

  const selectedProduct = (products.data ?? []).find((product) => product.id === selectedProductID) ?? null;
  const selectedSellingPoint = (sellingPoints.data ?? []).find((item) => item.id === selectedSellingPointID) ?? null;
  const productRows = (products.data ?? []).map((product) => ({ ...product, stats: productStatsMap[product.id] }));

  const openCreateProduct = () => {
    setEditingProduct(null);
    productForm.resetFields();
    setProductOpen(true);
  };

  const openEditProduct = (product: Product) => {
    setEditingProduct(product);
    productForm.setFieldsValue({
      name: product.name,
      category: product.category,
      description: product.description,
      reference_image: productReferenceImage(product)
    });
    setProductOpen(true);
  };

  const saveProduct = async () => {
    const values = await productForm.validateFields();
    const { reference_image: referenceImage, ...productValues } = values;
    const metadata = { ...(editingProduct?.metadata ?? {}) };
    if (referenceImage) {
      metadata.reference_image = referenceImage;
    } else {
      delete metadata.reference_image;
    }
    await persistProduct(editingProduct?.id, { ...productValues, metadata }, token);
    setProductOpen(false);
    setEditingProduct(null);
    productForm.resetFields();
    await products.reload();
    message.success(editingProduct ? "产品已更新" : "产品已创建");
  };

  const deleteProduct = async (product: Product) => {
    if ((productStatsMap[product.id]?.asset_count ?? 0) > 0) {
      message.warning("该产品已关联素材，不允许删除");
      return;
    }
    await removeProduct(product.id, token);
    if (selectedProductID === product.id) {
      setSelectedProductID(null);
      setSelectedSellingPointID(null);
    }
    await products.reload();
    message.success("产品已删除");
  };

  const openSellingPointManager = (productID: string) => {
    setSelectedProductID(productID);
    setSelectedSellingPointID(null);
    setSellingPointManagerOpen(true);
  };

  const openCreateSellingPoint = () => {
    setEditingSellingPoint(null);
    sellingPointForm.resetFields();
    setSellingPointOpen(true);
  };

  const openEditSellingPoint = (sellingPoint: SellingPoint) => {
    setEditingSellingPoint(sellingPoint);
    sellingPointForm.setFieldsValue({
      title: sellingPoint.title,
      priority: sellingPoint.priority,
      description: sellingPoint.description
    });
    setSellingPointOpen(true);
  };

  const saveSellingPoint = async () => {
    if (!selectedProductID) {
      message.warning("请先选择产品");
      return;
    }
    const values = await sellingPointForm.validateFields();
    await persistSellingPoint(editingSellingPoint?.id, selectedProductID, values, token);
    setSellingPointOpen(false);
    setEditingSellingPoint(null);
    sellingPointForm.resetFields();
    await sellingPoints.reload();
    message.success(editingSellingPoint ? "卖点已更新" : "卖点已创建");
  };

  const deleteSellingPoint = async (sellingPoint: SellingPoint) => {
    if ((sellingPoint.asset_count ?? 0) > 0) {
      message.warning("该卖点已关联素材，不允许删除");
      return;
    }
    await removeSellingPoint(sellingPoint.id, token);
    if (selectedSellingPointID === sellingPoint.id) {
      setSelectedSellingPointID(null);
    }
    await sellingPoints.reload();
    message.success("卖点已删除");
  };

  const openSellingPointAssets = (sellingPointID: string) => {
    setSelectedSellingPointID(sellingPointID);
    setSellingPointAssetsOpen(true);
  };

  return (
    <Space direction="vertical" size="middle" className="page-stack">
      <Typography.Title level={3}>产品</Typography.Title>
      <Card title="产品列表" extra={<Button type="primary" onClick={openCreateProduct}>新建产品</Button>}>
        <Table<(Product & { stats?: ProductStats })>
          rowKey="id"
          loading={products.loading || productStatsLoading}
          dataSource={productRows}
          columns={[
            {
              title: "参考图",
              width: 88,
              render: (_, record) =>
                productReferenceImage(record) ? (
                  <img className="product-reference-thumb" src={productReferenceImage(record)} alt={`${record.name}参考图`} />
                ) : (
                  <Typography.Text type="secondary">未上传</Typography.Text>
                )
            },
            { title: "产品", dataIndex: "name", width: 220 },
            { title: "分类", dataIndex: "category", render: (value) => value || "-" },
            { title: "状态", dataIndex: "status", width: 100, render: (status) => <Tag>{translateValue(status, productStatusLabels)}</Tag> },
            { title: "素材", width: 90, render: (_, record) => record.stats?.asset_count ?? "-" },
            { title: "可用素材", width: 100, render: (_, record) => record.stats?.usable_asset_count ?? "-" },
            { title: "待分析", width: 90, render: (_, record) => record.stats?.pending_analysis_count ?? "-" },
            {
              title: "操作",
              key: "actions",
              width: 300,
              render: (_, record) => (
                <Space size="small" wrap>
                  <Button type="link" className="table-link-button" onClick={() => openSellingPointManager(record.id)}>
                    卖点
                  </Button>
                  <Button type="link" className="table-link-button" onClick={() => openEditProduct(record)}>
                    编辑
                  </Button>
                  <Popconfirm
                    title="删除产品"
                    description="没有素材关联的产品会被直接删除。确认删除？"
                    okText="删除"
                    cancelText="取消"
                    onConfirm={() => void deleteProduct(record)}
                  >
                    <Button type="link" danger className="table-link-button">
                      删除
                    </Button>
                  </Popconfirm>
                </Space>
              )
            }
          ]}
        />
      </Card>

      <ProductEditorModal
        open={productOpen}
        editingProduct={editingProduct}
        form={productForm}
        onSave={() => void saveProduct()}
        onCancel={() => {
          setProductOpen(false);
          setEditingProduct(null);
          productForm.resetFields();
        }}
        onError={message.error}
      />

      <Modal
        title={selectedProduct ? `卖点管理：${selectedProduct.name}` : "卖点管理"}
        open={sellingPointManagerOpen}
        onCancel={() => {
          setSellingPointManagerOpen(false);
          setSelectedSellingPointID(null);
        }}
        footer={null}
        width={980}
      >
        <Space direction="vertical" size="middle" className="wide-space">
          <div className="products-modal-toolbar">
            <Typography.Text type="secondary">管理当前产品卖点，查看每个卖点关联的素材画面。</Typography.Text>
            <Button type="primary" disabled={!selectedProductID} onClick={openCreateSellingPoint}>
              新建卖点
            </Button>
          </div>
          <Table<SellingPoint>
            rowKey="id"
            loading={sellingPoints.loading}
            dataSource={selectedProductID ? sellingPoints.data ?? [] : []}
            pagination={false}
            locale={{ emptyText: "当前产品还没有卖点" }}
            columns={[
              { title: "标题", dataIndex: "title" },
              { title: "优先级", dataIndex: "priority", width: 100 },
              { title: "关联素材", dataIndex: "asset_count", width: 110, render: (value) => value ?? 0 },
              { title: "状态", dataIndex: "status", width: 100, render: (status) => <Tag>{translateValue(status, productStatusLabels)}</Tag> },
              {
                title: "操作",
                key: "actions",
                width: 260,
                render: (_, record) => (
                  <Space size="small" wrap>
                    <Button type="link" className="table-link-button" onClick={() => openSellingPointAssets(record.id)}>
                      关联素材
                    </Button>
                    <Button type="link" className="table-link-button" onClick={() => openEditSellingPoint(record)}>
                      编辑
                    </Button>
                    <Popconfirm
                      title="删除卖点"
                      description="没有素材关联的卖点会被直接删除。确认删除？"
                      okText="删除"
                      cancelText="取消"
                      onConfirm={() => void deleteSellingPoint(record)}
                    >
                      <Button type="link" danger className="table-link-button">
                        删除
                      </Button>
                    </Popconfirm>
                  </Space>
                )
              }
            ]}
          />
        </Space>
      </Modal>

      <Modal
        title={editingSellingPoint ? "编辑卖点" : selectedProduct ? `新建卖点：${selectedProduct.name}` : "新建卖点"}
        open={sellingPointOpen}
        onOk={saveSellingPoint}
        onCancel={() => {
          setSellingPointOpen(false);
          setEditingSellingPoint(null);
          sellingPointForm.resetFields();
        }}
        okText="确认"
        cancelText="取消"
      >
        <Form form={sellingPointForm} layout="vertical">
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

      <Drawer
        title={selectedSellingPoint ? `关联素材：${selectedSellingPoint.title}` : "关联素材"}
        open={sellingPointAssetsOpen}
        onClose={() => setSellingPointAssetsOpen(false)}
        width={980}
      >
        {sellingPointAssets.loading ? (
          <Card loading />
        ) : (sellingPointAssets.data ?? []).length === 0 ? (
          <Empty description="当前卖点还没有关联素材" />
        ) : (
          <div className="product-linked-asset-grid">
            {(sellingPointAssets.data ?? []).map((asset) => (
              <Card key={asset.id} className="product-linked-asset-card" bodyStyle={{ padding: 12 }}>
                <video className="product-linked-asset-video" src={assetVideoURL(asset)} muted preload="metadata" controls />
                <Typography.Text className="product-linked-asset-title" title={assetDisplayTitle(asset)}>
                  {assetDisplayTitle(asset)}
                </Typography.Text>
                <Space size={[4, 4]} wrap>
                  <Tag>{translateValue(asset.source_type, sourceTypeLabels)}</Tag>
                  <Tag>{formatDuration(asset.duration_ms)}</Tag>
                  {asset.width && asset.height ? <Tag>{asset.width}x{asset.height}</Tag> : null}
                  {asset.shot_size ? <Tag>{translateValue(asset.shot_size, shotSizeLabels)}</Tag> : null}
                  {asset.camera_movement ? <Tag>{translateValue(asset.camera_movement, cameraMovementLabels)}</Tag> : null}
                </Space>
                <Typography.Paragraph className="product-linked-asset-desc" ellipsis={{ rows: 2 }}>
                  {asset.scene_description || "暂无画面描述"}
                </Typography.Paragraph>
              </Card>
            ))}
          </div>
        )}
      </Drawer>
    </Space>
  );
}
