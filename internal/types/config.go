package types

type DifySandboxGlobalConfigurations struct {
	App struct {
		Port       int    `yaml:"port"`
		Debug      bool   `yaml:"debug"`
		GatewayKey string `yaml:"gateway_key"`
		AdminKey   string `yaml:"admin_key"`
		Key        string `yaml:"key"`
	} `yaml:"app"`
	MaxWorkers      int  `yaml:"max_workers"`
	MaxRequests     int  `yaml:"max_requests"`
	WorkerTimeout   int  `yaml:"worker_timeout"`
	EnableNetwork   bool `yaml:"enable_network"`
	EnablePreload   bool `yaml:"enable_preload"`
	AllowedSyscalls []int `yaml:"allowed_syscalls"`
	Proxy struct {
		Socks5 string `yaml:"socks5"`
		Https  string `yaml:"https"`
		Http   string `yaml:"http"`
	} `yaml:"proxy"`
	
	// 添加缺失的配置字段
	Gateway struct {
		Port                int    `yaml:"port"`
		LoadBalancerStrategy string `yaml:"load_balancer_strategy"`
		HealthCheckInterval int    `yaml:"health_check_interval"`
		CorsEnabled         bool   `yaml:"cors_enabled"`
	} `yaml:"gateway"`
	
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"redis"`
}