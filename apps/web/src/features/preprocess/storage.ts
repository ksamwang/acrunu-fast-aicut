const LAST_SUBMIT_PRODUCT_STORAGE_KEY = "aicut.preprocess.last_submit_product_id";

export function loadLastSubmitProductID() {
  if (typeof window === "undefined") {
    return "";
  }
  try {
    return window.localStorage.getItem(LAST_SUBMIT_PRODUCT_STORAGE_KEY) ?? "";
  } catch {
    return "";
  }
}

export function persistLastSubmitProductID(productID: string) {
  if (typeof window === "undefined" || !productID) {
    return;
  }
  try {
    window.localStorage.setItem(LAST_SUBMIT_PRODUCT_STORAGE_KEY, productID);
  } catch {
    // A blocked storage API should not prevent preprocessing or submission.
  }
}
