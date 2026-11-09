// genicon 生成 PWA 应用图标（渐变圆角方块 + 白色 ¥ 符号）
// 用法: go run ./tools/genicon <size> <out.png>
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strconv"
)

var white = color.RGBA{255, 255, 255, 255}

func lerp(a, b, t float64) uint8 {
	return uint8(a + (b-a)*t)
}

func inRounded(x, y, size, rad int) bool {
	if x < 0 || y < 0 || x >= size || y >= size {
		return false
	}
	cx := []int{rad, size - rad - 1}
	cy := []int{rad, size - rad - 1}
	minx, miny := rad, rad
	maxx, maxy := size-rad-1, size-rad-1
	if x >= minx && x <= maxx || y >= miny && y <= maxy {
		return true
	}
	dx := math.Min(math.Abs(float64(x-cx[0])), math.Abs(float64(x-cx[1])))
	dy := math.Min(math.Abs(float64(y-cy[0])), math.Abs(float64(y-cy[1])))
	return dx*dx+dy*dy <= float64(rad)*float64(rad)
}

// thickLine 用点到线段距离画粗线
func thickLine(img *image.NRGBA, x1, y1, x2, y2, w int, c color.RGBA) {
	hw := float64(w) / 2
	sx, sy := float64(x1), float64(y1)
	ex, ey := float64(x2), float64(y2)
	dx, dy := ex-sx, ey-sy
	len2 := dx*dx + dy*dy
	lo := image.Point{X: int(math.Min(sx, ex) - hw - 1), Y: int(math.Min(sy, ey) - hw - 1)}
	hi := image.Point{X: int(math.Max(sx, ex) + hw + 1), Y: int(math.Max(sy, ey) + hw + 1)}
	for py := lo.Y; py <= hi.Y; py++ {
		for px := lo.X; px <= hi.X; px++ {
			if px < 0 || py < 0 || px >= img.Bounds().Dx() || py >= img.Bounds().Dy() {
				continue
			}
			fx, fy := float64(px)+0.5, float64(py)+0.5
			var t float64
			if len2 == 0 {
				t = 0
			} else {
				t = ((fx-sx)*dx + (fy-sy)*dy) / len2
				t = math.Max(0, math.Min(1, t))
			}
			qx, qy := sx+t*dx, sy+t*dy
			if (fx-qx)*(fx-qx)+(fy-qy)*(fy-qy) <= hw*hw {
				img.Set(px, py, c)
			}
		}
	}
}

func hBar(img *image.NRGBA, x1, x2, y, w int, c color.RGBA) {
	thickLine(img, x1, y, x2, y, w, c)
}

func main() {
	size, _ := strconv.Atoi(os.Args[1])
	out := os.Args[2]
	rad := size * 22 / 100
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !inRounded(x, y, size, rad) {
				continue
			}
			t := float64(x+y) / float64(2*size)
			img.Set(x, y, color.RGBA{
				lerp(0x6a, 0x3a, t),
				lerp(0x83, 0x56, t),
				lerp(0xff, 0xe0, t),
				255,
			})
		}
	}
	w := size * 7 / 100
	cx := size / 2
	// ¥：两撇 + 竖 + 两横
	thickLine(img, size*30/100, size*20/100, cx, size*46/100, w, white)
	thickLine(img, size*70/100, size*20/100, cx, size*46/100, w, white)
	thickLine(img, cx, size*44/100, cx, size*78/100, w, white)
	hBar(img, size*29/100, size*71/100, size*54/100, w, white)
	hBar(img, size*29/100, size*71/100, size*67/100, w, white)

	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
