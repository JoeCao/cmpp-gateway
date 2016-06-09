package gateway

import (
	"fmt"
	//"strconv"
	"testing"
)

func TestInsert(t *testing.T) {
	list := New()
	list.PushFront("1")
	list.PushFront("2")
	list.PushFront("3")
	list.PushFront("4")

	fmt.Println(list.Front())
	fmt.Println("first next :", list.Front())
	fmt.Println("first prev :", list.Back())

	//fmt.Println(list.head)
	//fmt.Println(list.last.prev)
	//for i, value := range list {
	//	fmt.Println(strconv.FormatUint(i, 10) + ":" + value)
	//}

}
