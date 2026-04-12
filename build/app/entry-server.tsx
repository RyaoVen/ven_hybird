import React from "react";

interface AppProps {
    route: string;
    data?: unknown;
}

export default function App({ route, data }: AppProps) {
    return (
        <div>
            <h1>Page: {route}</h1>
            <pre>{JSON.stringify(data, null, 2)}</pre>
        </div>
    );
}

export async function getInitialProps(ctx: unknown) {
    return {
        title: "Home Page",
        data: ctx,
    };
}
