package cache

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sb "github.com/open-lambda/open-lambda/worker/sandbox"

	"github.com/open-lambda/open-lambda/worker/config"
)

type CacheManager struct {
	factory CacheFactory
	cluster string
	servers []*ForkServer
	matcher CacheMatcher
	seq     int
	mutex   *sync.Mutex
	sizes   map[string]float64
	full    *int32
}

func InitCacheManager(opts *config.Config) (cm *CacheManager, err error) {
	if opts.Import_cache_size == 0 {
		return nil, nil
	}

	servers := make([]*ForkServer, 0, 0)
	sizes, err := readPkgSizes("/ol/open-lambda/worker/cache-manager/package_sizes.txt")
	if err != nil {
		return nil, err
	}

	var full int32 = 0
	cm = &CacheManager{
		cluster: opts.Cluster_name,
		servers: servers,
		matcher: NewSubsetMatcher(),
		seq:     0,
		mutex:   &sync.Mutex{},
		sizes:   sizes,
		full:    &full,
	}

	memCGroupPath, err := cm.initCacheRoot(opts)
	if err != nil {
		return nil, err
	}

	e, err := NewEvictor(cm, "", memCGroupPath, opts.Import_cache_size)
	if err != nil {
		return nil, err
	}

	go func(cm *CacheManager) {
		for {
			time.Sleep(50 * time.Millisecond)
			e.CheckUsage()
		}
	}(cm)

	return cm, nil
}

func (cm *CacheManager) Provision(sandbox sb.ContainerSandbox, dir string, pkgs []string) (fs *ForkServer, hit bool, err error) {
	cm.mutex.Lock()

	fs, toCache, hit := cm.matcher.Match(cm.servers, pkgs)

	// make new cache entry if necessary
	if len(toCache) != 0 {
		fs, err = cm.newCacheEntry(fs, toCache)
		if err != nil {
			return nil, false, err
			//return cm.Provision(sandbox, dir, pkgs) //TODO
		}
	} else {
		fs.Mutex.Lock()
		cm.mutex.Unlock()
		if fs == nil {
			fs.Mutex.Unlock()
			return nil, false, err
			//return cm.Provision(sandbox, dir, pkgs) //TODO
		}
	}
	defer fs.Mutex.Unlock()

	// keep track of number of hits
	fs.Hit()

	// signal interpreter to forkenter into sandbox's namespace
	pid, err := forkRequest(fs.SockPath, sandbox.NSPid(), sandbox.RootDir(), []string{}, true)
	if err != nil {
		return nil, false, err
	}

	// change cgroup of spawned lambda server
	if err = sandbox.CGroupEnter(pid); err != nil {
		return nil, false, err
	}

	return fs, hit, nil
}

func (cm *CacheManager) newCacheEntry(fs *ForkServer, toCache []string) (*ForkServer, error) {
	// make hashset of packages for new entry
	pkgs := make(map[string]bool)
	size := 0.0
	for key, val := range fs.Packages {
		pkgs[key] = val
	}
	for k := 0; k < len(toCache); k++ {
		pkgs[toCache[k]] = true
		size += cm.sizes[strings.ToLower(toCache[k])]
	}

	newFs := &ForkServer{
		Packages: pkgs,
		Hits:     0.0,
		Parent:   fs,
		Children: 0,
		Mutex:    &sync.Mutex{},
	}

	fs.Children += 1

	cm.servers = append(cm.servers, newFs)
	cm.seq++

	newFs.Mutex.Lock()
	cm.mutex.Unlock()

	// get container for new entry
	sandbox, err := cm.factory.Create([]string{"/init"})
	if err != nil {
		newFs.Kill()
		return nil, err
	}

	// signal interpreter to forkenter into sandbox's namespace
	pid, err := forkRequest(fs.SockPath, sandbox.NSPid(), sandbox.RootDir(), toCache, false)
	if err != nil {
		newFs.Mutex.Unlock()
		newFs.Kill()
		return nil, err
	}

	sockPath := fmt.Sprintf("%s/fs.sock", sandbox.HostDir())

	// use StdoutPipe of olcontainer to sync with lambda server
	ready := make(chan bool, 1)
	go func() {
		pipeDir := filepath.Join(sandbox.HostDir(), "pipe")
		pipe, err := os.OpenFile(pipeDir, os.O_RDWR, 0777)
		if err != nil {
			log.Fatalf("Cannot open pipe: %v\n", err)
		}
		defer pipe.Close()

		// wait for "ready"
		buf := make([]byte, 5)
		n, err := pipe.Read(buf)
		if err != nil {
			log.Fatalf("Cannot read from stdout of olcontainer: %v\n", err)
		} else if n != 5 {
			log.Fatalf("Expect to read 5 bytes, only %d read\n", n)
		}
		ready <- true
	}()

	// wait up to 20s for server to initialize
	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(20 * time.Second)
		timeout <- true
	}()

	// wait up to 30s for server to initialize
	start := time.Now()
	select {
	case <-ready:
		log.Printf("wait for server took %v\n", time.Since(start))
	case <-timeout:
		return nil, fmt.Errorf("handler server failed to initialize after 20s")
	}

	newFs.Sandbox = sandbox
	newFs.Pid = pid
	newFs.SockPath = sockPath

	return newFs, nil
}

func (cm *CacheManager) initCacheRoot(opts *config.Config) (memCGroupPath string, err error) {
	factory, rootSB, rootDir, err := InitCacheFactory(opts, cm.cluster)
	if err != nil {
		return "", err
	}
	cm.factory = factory

	// wait up to 5s for root server to spawn
	sockPath := fmt.Sprintf("%s/fs.sock", rootDir)
	start := time.Now()
	for ok := true; ok; ok = os.IsNotExist(err) {
		_, err = os.Stat(sockPath)
		if time.Since(start).Seconds() > 5 {
			return "", errors.New("root forkserver failed to start after 5s")
		}
	}

	fs := &ForkServer{
		Sandbox:  rootSB,
		Pid:      "-1",
		SockPath: fmt.Sprintf("%s/fs.sock", rootDir),
		Packages: make(map[string]bool),
		Hits:     0.0,
		Parent:   nil,
		Children: 0,
		Mutex:    &sync.Mutex{},
		Size:     1.0, // divide-by-zero
	}

	cm.servers = append(cm.servers, fs)

	return rootSB.MemoryCGroupPath(), nil
}

func (cm *CacheManager) Full() bool {
	return atomic.LoadInt32(cm.full) == 1
}

func readPkgSizes(path string) (map[string]float64, error) {
	sizes := make(map[string]float64)
	file, err := os.Open(path)
	if err != nil {
		log.Printf("invalid package sizes path %v, using 0 for all", path)
		return make(map[string]float64), nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if err = scanner.Err(); err != nil {
			return nil, err
		}

		split := strings.Split(scanner.Text(), ":")
		if len(split) != 2 {
			return nil, errors.New("malformed package size file")
		}

		size, err := strconv.Atoi(split[1])
		if err != nil {
			return nil, err
		}
		sizes[strings.ToLower(split[1])] = float64(size)
	}

	return sizes, nil
}

func (cm *CacheManager) Cleanup() {
	for _, server := range cm.servers {
		server.Kill()
	}

	cm.factory.Cleanup()
}
