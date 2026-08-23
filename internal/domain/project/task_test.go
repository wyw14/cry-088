package project

import "testing"

func TestValidateDependencyGraph(t *testing.T) {
	tests := []struct {
		name    string
		tasks   []Task
		wantErr bool
	}{
		{name: "acyclic", tasks: []Task{{ID: "a"}, {ID: "b", DependencyIDs: []string{"a"}}}},
		{name: "cycle", tasks: []Task{{ID: "a", DependencyIDs: []string{"b"}}, {ID: "b", DependencyIDs: []string{"a"}}}, wantErr: true},
		{name: "missing", tasks: []Task{{ID: "a", DependencyIDs: []string{"missing"}}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateDependencyGraph(tt.tasks); (err != nil) != tt.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
