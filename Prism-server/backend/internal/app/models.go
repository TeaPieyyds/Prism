package app

type enterTarget struct {
	Name  string
	Label string
	Hint  string
}

var targets = []enterTarget{
	{Name: "rental", Label: "租赁服 (Rental Game)", Hint: "请输入服务器号/名称"},
	{Name: "domain", Label: "我的山头 (Domain Game)", Hint: "请输入邀请码"},
	{Name: "tan", Label: "本地联机 (Tan Lobby)", Hint: "请输入房间号"},
	{Name: "online", Label: "联机大厅 (Online Lobby)", Hint: "请输入大厅房间号/关键词"},
	{Name: "network", Label: "网络游戏 (Network Game)", Hint: "请输入网络游戏编号"},
	{Name: "main_city", Label: "主城乐园 (Main City)", Hint: "请输入主城 ID(可随便填 1)"},
}
