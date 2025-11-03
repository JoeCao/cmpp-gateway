#!/bin/bash
# CMPP Gateway v2.0.0 发布打包脚本

set -e

VERSION="2.0.0"
BUILD_DIR="releases"
WORK_DIR=$(pwd)

echo "开始构建 CMPP Gateway v${VERSION} 发布包..."

# 清理旧的构建文件
rm -rf ${BUILD_DIR}/build_*

# 创建临时构建目录
mkdir -p ${BUILD_DIR}/build_temp

    # 构建函数
build_platform() {
    local OS=$1
    local ARCH=$2
    local EXT=$3
    local OUTPUT_NAME="cmpp-gateway-v${VERSION}-${OS}-${ARCH}"
    
    echo "正在构建 ${OS}-${ARCH}..."
    
    # 创建平台特定目录
    PLATFORM_DIR="${BUILD_DIR}/build_temp/cmpp-gateway-v${VERSION}-${OS}-${ARCH}"
    mkdir -p ${PLATFORM_DIR}
    
    # 交叉编译
    if [ "$OS" == "windows" ]; then
        GOOS=${OS} GOARCH=${ARCH} go build -mod=vendor -ldflags="-s -w" -o ${PLATFORM_DIR}/cmpp-gateway.exe
    else
        GOOS=${OS} GOARCH=${ARCH} go build -mod=vendor -ldflags="-s -w" -o ${PLATFORM_DIR}/cmpp-gateway
    fi
    
    # 复制模板文件
    cp -r templates ${PLATFORM_DIR}/
    
    # 复制配置文件示例
    cp config.boltdb.json ${PLATFORM_DIR}/config.json.example
    cp config.redis.json ${PLATFORM_DIR}/config.redis.json.example
    
    # 复制文档
    cp README.md ${PLATFORM_DIR}/
    
    # 创建压缩包
    cd ${BUILD_DIR}/build_temp
    tar -czf ${OUTPUT_NAME}.tar.gz -C . cmpp-gateway-v${VERSION}-${OS}-${ARCH}
    
    # 移动到 releases 目录
    mv ${OUTPUT_NAME}.tar.gz ../${OUTPUT_NAME}.tar.gz
    
    cd ${WORK_DIR}
    
    echo "✓ ${OUTPUT_NAME}.tar.gz 构建完成"
}

# 构建所有平台
build_platform "linux" "amd64" ""
build_platform "linux" "arm64" ""
build_platform "windows" "amd64" ".exe"
build_platform "darwin" "amd64" ""
build_platform "darwin" "arm64" ""

# 清理临时文件
rm -rf ${BUILD_DIR}/build_temp

echo ""
echo "🎉 所有平台构建完成！"
echo ""
echo "构建文件："
ls -lh ${BUILD_DIR}/cmpp-gateway-v${VERSION}-*.tar.gz

echo ""
echo "构建完成时间: $(date)"
