package prom_op

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// GetClusterCPUUsage: Prometheus API를 사용하여 클러스터의 CPU 사용량을 가져오는 함수
func GetClusterCPUUsage(cluster string, prometheusURL string) (int, error) {
	// Prometheus 클라이언트 생성
	client, err := api.NewClient(api.Config{
		Address: prometheusURL, // Prometheus 서버 URL
	})
	if err != nil {
		return 0, fmt.Errorf("error creating Prometheus client: %v", err)
	}

	// Prometheus API 인터페이스
	v1api := v1.NewAPI(client)

	// CPU 사용량에 대한 Prometheus 쿼리 생성
	query := fmt.Sprintf(`avg(rate(container_cpu_usage_seconds_total{cluster="%s"}[5m]))`, cluster)

	// 현재 시간 기준으로 쿼리 수행
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, warnings, err := v1api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("error querying Prometheus: %v", err)
	}
	if len(warnings) > 0 {
		log.Printf("Warnings: %v", warnings)
	}

	// 결과를 처리하여 CPU 사용량을 리턴
	if result.Type() == model.ValVector {
		vector := result.(model.Vector)
		if len(vector) > 0 {
			// CPU 사용량을 퍼센트로 반환 (0~100 범위로 가정)
			cpuUsage := vector[0].Value * 100
			return int(cpuUsage), nil
		}
	}

	return 0, fmt.Errorf("no data found for cluster %s", cluster)
}
