package engine

import "testing"

// 精灵缓存必须有上限。
//
// 键里含颜色：页面若逐帧改圆角盒的颜色（动画），会不断产生新键；无上限的
// 缓存就是内存泄漏——旧精灵的像素一直没人回收。
func TestSpriteCacheIsBounded(t *testing.T) {
	isolateSpriteCache(t)

	// 逐帧换色：每次都是一个新键。
	for i := 0; i < maxSpriteItems+100; i++ {
		cornerSprite(8, NewColor(uint8(i), uint8(i>>8), uint8(i>>16), 255), false, false)
	}
	if len(spriteCache) > maxSpriteItems {
		t.Fatalf("精灵缓存条目 = %d, want <= %d", len(spriteCache), maxSpriteItems)
	}
	if spriteCacheBytes > maxSpriteBytes {
		t.Fatalf("精灵缓存字节 = %d, want <= %d", spriteCacheBytes, maxSpriteBytes)
	}
}

// 淘汰必须按最近最少使用：页面上复用最多的精灵不该被先淘汰掉。
//
// FIFO 会把复用最多的先淘汰——工作集略大于上限时每帧都要重建精灵（重建 +
// 后端重新上传纹理），缓存颠簸。
func TestSpriteCacheEvictsLeastRecentlyUsed(t *testing.T) {
	isolateSpriteCache(t)

	hot := NewColor(1, 2, 3, 255)
	hotSprite := cornerSprite(8, hot, false, false)
	for i := 0; i < maxSpriteItems+100; i++ {
		cornerSprite(8, NewColor(uint8(i), uint8(i>>8), 0, 255), false, false)
		cornerSprite(8, hot, false, false) // 保持热度
	}
	if got := cornerSprite(8, hot, false, false); got != hotSprite {
		t.Fatal("热点精灵被淘汰了（LRU 失效：应当保留最近最少使用之外的那些）")
	}
}

// isolateSpriteCache 把全局精灵缓存换成空表（测试结束还原，免得影响别的用例）。
func isolateSpriteCache(t *testing.T) {
	t.Helper()
	savedCache := spriteCache
	savedClock := spriteCacheClock
	savedBytes := spriteCacheBytes
	spriteCache = map[spriteKey]*spriteEntry{}
	spriteCacheClock = 0
	spriteCacheBytes = 0
	t.Cleanup(func() {
		spriteCache = savedCache
		spriteCacheClock = savedClock
		spriteCacheBytes = savedBytes
	})
}
