package workload

// Query constants and helpers will live here. For now this file declares the
// canonical Cypher queries used by the harness.

const (
	PointLookupQuery  = "MATCH (u:User {id:$id}) RETURN u LIMIT 1"
	TraversalQueryFmt = "MATCH (u:User {id:$id})-[:MUTUAL_FOLLOW*%d]->(m) RETURN DISTINCT m LIMIT 100"
	AggregationQuery  = "MATCH (u:User)-[r:MUTUAL_FOLLOW]->() RETURN u.id, COUNT(r) AS degree ORDER BY degree DESC LIMIT 10"
)
