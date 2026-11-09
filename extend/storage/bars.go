package storage

import "github.com/injoyai/tdx/v2/extend/model"

// BarStore 是行情持久化边界，不要求 SQL、文件或指定 ORM。
// 所有方法须支持并发调用，并严格按 Code.Market/Code.Code 隔离证券。
// Identity 是稳定的存储命名空间，用于任务去重；不同数据集不能共用身份，不能包含密码。
// 查询按源时间升序返回，空端点不限范围；无行情返回空、无最后时间返回空字符串。
// SaveDay/SaveMinute 原子删除指定证券 Time>=from 的尾部后插入 rows；分钟只影响指定年。
// coverage 非空时，分钟数据与覆盖标记须原子提交，并合并相容的既有覆盖范围。
// 空数据不删除行情或写完成标记。注入的存储由调用方关闭。
type BarStore interface {
	// 完成标记使用 Unix 秒，独立于行情源时间；pull 保存本轮已处理的更新窗口边界。
	// 缺少记录时返回 has=false。
	// LoadUpdate 只读；SaveUpdate 须支持首次写入和覆盖已有标记。
	LoadUpdate(key string) (unix int64, has bool, err error)
	SaveUpdate(key string, unix int64) error
	Identity() string
	LastDayTime(model.Code) (string, error)
	QueryDay(code model.Code, start, end string) ([]*model.KlineDay, error)
	SaveDay(code model.Code, from string, rows []*model.KlineDay) error
	LastMinuteTime(code model.Code, year int) (string, error)
	QueryMinute(code model.Code, start, end string) ([]*model.KlineMinute, error)
	SaveMinute(code model.Code, year int, from string, rows []*model.KlineMinute, coverage *model.MinuteCoverage) error
	QueryMinuteCoverage(code model.Code, year int) (*model.MinuteCoverage, error)
	LoadMinuteEmptyScan(code model.Code, year int) (*model.MinuteEmptyScan, error)
	SaveMinuteEmptyScan(code model.Code, year int, scan model.MinuteEmptyScan) error
}
