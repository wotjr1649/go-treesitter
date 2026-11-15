-- Open rentals and their customers, as used by an application report.
SELECT c.customer_id, c.first_name, c.last_name, count(*) AS open_rentals
FROM customer AS c
JOIN rental AS r USING (customer_id)
WHERE r.return_date IS NULL
GROUP BY c.customer_id, c.first_name, c.last_name
HAVING count(*) > 1
ORDER BY open_rentals DESC, c.customer_id
LIMIT 20;
