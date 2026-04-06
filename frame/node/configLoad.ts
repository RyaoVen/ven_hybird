import * as fs from "node:fs";
import * as path from "node:path";

/**
 * 配置加载器
 * 默认读取 frame 目录下的 config.yaml 或 config.toml 文件
 * 支持 YAML 和 TOML 两种格式
 */

/** 配置文件格式类型 */
type ConfigFormat = "yaml" | "toml";

/** 配置加载选项 */
interface ConfigLoadOptions {
    /** 配置文件目录，默认为 frame 目录 */
    configDir?: string;
    /** 配置文件名（不含扩展名），默认为 "config" */
    configName?: string;
    /** 优先格式，默认自动检测 */
    preferredFormat?: ConfigFormat;
}

/** 缓存的配置对象 */
let cachedConfig: Record<string, unknown> | null = null;

/** 已加载的配置文件路径 */
let loadedConfigPath: string | null = null;

/** 已加载的配置格式 */
let loadedFormat: ConfigFormat | null = null;

/**
 * 解析 YAML 内容
 */
function parseYaml(content: string): Record<string, unknown> {
    // 动态导入 yaml 包
    const YAML = require("yaml");
    return YAML.parse(content) as Record<string, unknown>;
}

/**
 * 解析 TOML 内容
 */
function parseToml(content: string): Record<string, unknown> {
    // 动态导入 @iarna/toml 包
    const TOML = require("@iarna/toml");
    return TOML.parse(content) as Record<string, unknown>;
}

/**
 * 检测配置文件格式并返回文件路径
 */
function detectConfigFile(
    configDir: string,
    configName: string,
    preferredFormat?: ConfigFormat
): { path: string; format: ConfigFormat } | null {
    const extensions: Record<ConfigFormat, string> = {
        yaml: ".yaml",
        toml: ".toml",
    };

    // 如果指定了优先格式，先尝试该格式
    if (preferredFormat) {
        const filePath = path.join(configDir, `${configName}${extensions[preferredFormat]}`);
        if (fs.existsSync(filePath)) {
            return { path: filePath, format: preferredFormat };
        }
        // 也尝试 .yml 扩展名（yaml 的另一种写法）
        if (preferredFormat === "yaml") {
            const ymlPath = path.join(configDir, `${configName}.yml`);
            if (fs.existsSync(ymlPath)) {
                return { path: ymlPath, format: "yaml" };
            }
        }
    }

    // 自动检测，按优先级：yaml > toml
    const searchOrder: ConfigFormat[] = ["yaml", "toml"];

    for (const format of searchOrder) {
        const filePath = path.join(configDir, `${configName}${extensions[format]}`);
        if (fs.existsSync(filePath)) {
            return { path: filePath, format };
        }

        // yaml 也支持 .yml 扩展名
        if (format === "yaml") {
            const ymlPath = path.join(configDir, `${configName}.yml`);
            if (fs.existsSync(ymlPath)) {
                return { path: ymlPath, format: "yaml" };
            }
        }
    }

    return null;
}

/**
 * 加载并解析配置文件
 * @param options 配置加载选项
 * @returns 解析后的配置对象
 */
function loadConfig(options: ConfigLoadOptions = {}): Record<string, unknown> {
    // 默认配置目录为 frame 目录（相对于 node 目录的上级）
    const defaultConfigDir = path.resolve(__dirname, "..");
    const configDir = options.configDir ?? defaultConfigDir;
    const configName = options.configName ?? "config";

    // 检测配置文件
    const configFile = detectConfigFile(configDir, configName, options.preferredFormat);

    if (!configFile) {
        throw new Error(
            `配置文件不存在: 在 ${configDir} 目录下未找到 ${configName}.yaml 或 ${configName}.toml`
        );
    }

    // 如果已经缓存且路径相同，直接返回缓存
    if (cachedConfig && loadedConfigPath === configFile.path) {
        return cachedConfig;
    }

    // 读取文件内容
    const content = fs.readFileSync(configFile.path, "utf-8");

    // 根据格式解析
    let config: Record<string, unknown>;
    switch (configFile.format) {
        case "yaml":
            config = parseYaml(content);
            break;
        case "toml":
            config = parseToml(content);
            break;
        default:
            throw new Error(`不支持的配置格式: ${configFile.format}`);
    }

    // 缓存配置
    cachedConfig = config;
    loadedConfigPath = configFile.path;
    loadedFormat = configFile.format;

    return config;
}

/**
 * 获取配置值
 * 支持点分隔的路径访问，如 "server.port"
 * @param key 配置键，支持点分隔路径
 * @param defaultValue 默认值
 * @returns 配置值或默认值
 */
export function getConfig<T = unknown>(key?: string, defaultValue?: T): T {
    // 确保配置已加载
    if (!cachedConfig) {
        loadConfig();
    }

    if (!cachedConfig) {
        return defaultValue as T;
    }

    // 如果没有指定 key，返回整个配置对象
    if (!key) {
        return cachedConfig as T;
    }

    // 支持点分隔路径访问
    const parts = key.split(".");
    let value: unknown = cachedConfig;

    for (const part of parts) {
        if (value === null || value === undefined) {
            return defaultValue as T;
        }

        if (typeof value === "object" && part in (value as Record<string, unknown>)) {
            value = (value as Record<string, unknown>)[part];
        } else {
            return defaultValue as T;
        }
    }

    return (value ?? defaultValue) as T;
}

/**
 * 获取整个配置对象
 * @returns 配置对象
 */
export function getConfigAll(): Record<string, unknown> {
    if (!cachedConfig) {
        loadConfig();
    }
    return cachedConfig ?? {};
}

/**
 * 重新加载配置文件
 * 强制重新读取并解析配置文件
 * @param options 配置加载选项
 * @returns 新的配置对象
 */
export function reloadConfig(options: ConfigLoadOptions = {}): Record<string, unknown> {
    cachedConfig = null;
    loadedConfigPath = null;
    loadedFormat = null;
    return loadConfig(options);
}

/**
 * 获取已加载的配置文件信息
 */
export function getConfigFileInfo(): {
    path: string | null;
    format: ConfigFormat | null;
} {
    return {
        path: loadedConfigPath,
        format: loadedFormat,
    };
}

/**
 * 初始化配置加载器
 * 可在应用启动时调用，提前加载配置
 * @param options 配置加载选项
 */
export function initConfig(options: ConfigLoadOptions = {}): void {
    loadConfig(options);
}

// 默认导出 getConfig 函数
export default getConfig;