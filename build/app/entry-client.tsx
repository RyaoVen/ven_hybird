import React from "react";
import { hydrateRoot } from "react-dom/client";
import App from "./entry-server";

declare global {
    interface Window {
        __SPA_DATA__?: {
            route?: string;
            data?: unknown;
        };
    }
}

const initialData = window.__SPA_DATA__ ?? {};
const route = initialData.route ?? "/";
const data = initialData.data ?? {};
const root = document.getElementById("root");

if (root) {
    hydrateRoot(root, <App route={route} data={data} />);
}
