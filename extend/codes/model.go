package codes

import "github.com/injoyai/tdx/v2/internal/codecache"

// Interface 是不触发网络访问的代码表读取接口。
type Interface = codecache.Interface

// CodeModel 保留代码缓存的数据库字段及 HTTP JSON 结构。
type CodeModel = codecache.CodeModel

type CodeModels = codecache.CodeModels

// Base 是可独立使用的纯内存代码表。
type Base = codecache.Base

// NewBase 创建空的内存代码表，不联网、不打开数据库。
func NewBase() *Base { return codecache.NewBase() }
