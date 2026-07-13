import { apiRequest } from "../../shared/api/server-api";
import type { Asset } from "../../shared/types/asset";
import type { Product, ProductStats, SellingPoint } from "../../shared/types/product";

export const listProducts = (path: string, token: string) => apiRequest<Product[]>(path, {}, token);
export const listSellingPoints = (path: string, token: string) => apiRequest<SellingPoint[]>(path, {}, token);
export const listSellingPointAssets = (path: string, token: string) => apiRequest<Asset[]>(path, {}, token);
export const getProductStats = (productID: string, token: string) => apiRequest<ProductStats>("/api/products/" + productID + "/stats", {}, token);

export const saveProduct = (productID: string | undefined, values: unknown, token: string) =>
  apiRequest<Product>(productID ? "/api/products/" + productID : "/api/products", { method: productID ? "PUT" : "POST", body: JSON.stringify(values) }, token);

export const deleteProduct = (productID: string, token: string) => apiRequest("/api/products/" + productID, { method: "DELETE" }, token);

export const saveSellingPoint = (sellingPointID: string | undefined, productID: string, values: unknown, token: string) =>
  apiRequest<SellingPoint>(sellingPointID ? "/api/selling-points/" + sellingPointID : "/api/products/" + productID + "/selling-points", { method: sellingPointID ? "PUT" : "POST", body: JSON.stringify(values) }, token);

export const deleteSellingPoint = (sellingPointID: string, token: string) => apiRequest("/api/selling-points/" + sellingPointID, { method: "DELETE" }, token);
