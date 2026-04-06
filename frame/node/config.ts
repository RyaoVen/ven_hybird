
import type {SPAClientOptions} from "./pageBuild/SPAbuild";
import {SSRBuildOptions} from "./pageBuild/SSRbuild";

const SPAClientConfig:SPAClientOptions ={
    entryPoint: "../../../build/app/entry-client.tsx",
    minify:true,
    sourcemap:"external",
    external:[],
    format:"esm",
    write:true,
    outFile: "../../build/entry-client.js",
    publicPath:"/",
    target:["esnext"],
    loader:{
        ".tsx": "tsx",
        ".ts": "ts",
        ".jsx": "jsx",
        ".js": "js",
        ".css": "css",
        ".json": "json",
        ".png": "file",
        ".jpg": "file",
        ".jpeg": "file",
        ".gif": "file",
        ".svg": "file",
        ".ico": "file",
        ".webp": "file",
        ".mp4": "file",
        ".mp3": "file",
    }
}

const SSRBuildConfig:SSRBuildOptions ={
    entryPoint: "../../../build/app/entry-server.tsx",
    minify:true,
    sourcemap:"external",
    external:[],
    format:"cjs",
    write:true,
    outFile: "../../build/entry-server.js",
    target:["esnext"],
    loader:{
        ".tsx": "tsx",
        ".ts": "ts",
        ".jsx": "jsx",
        ".js": "js",
        ".css": "css",
        ".json": "json",
    }
}